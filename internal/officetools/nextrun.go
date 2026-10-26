package officetools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tung1998/agent-office/internal/storage"
)

// A scheduled run sets when its schedule runs next (ADR-202): a coordinator
// that just handed out quick work comes back sooner, one waiting on the real
// world later, instead of a fixed beat. One time only: the schedule's own
// timing resumes after that run. The run asked for goes even when the
// schedule's only_if finds nothing.

// Bounds of a next run asked for.
const (
	MinNextRun = 10 * time.Minute
	MaxNextRun = 7 * 24 * time.Hour
)

func nextRunTool() Tool {
	return Tool{Name: "schedule_next_run", Description: "In a run of a schedule (or the chat it keeps): set when that schedule runs next, once, from your estimate of when there will be something to do: sooner when the work you handed out is quick, later when waiting on the real world. Its usual timing resumes after that run. Between 10 minutes and 7 days.",
		Schema: obj(map[string]any{
			"minutes": map[string]any{"type": "integer", "description": "Run again in this many minutes (10-10080)"},
			"at":      map[string]any{"type": "string", "description": "Or at this time (RFC3339, e.g. 2026-10-26T14:30:00+07:00)"},
			"reason":  map[string]any{"type": "string", "description": "Your estimate: what should be ready by then"},
		}, "reason")}
}

// scheduleOf is the schedule sc's run belongs to: its job's automation, or
// the one that keeps sc's chat (a delegate's answer calls it again there).
func (t *Toolbox) scheduleOf(ctx context.Context, sc Scope) (storage.Automation, error) {
	if sc.JobID != "" {
		if j, err := t.store.Jobs().Get(ctx, sc.JobID); err == nil && j.Origin == "automation" && j.OriginID != "" {
			if a, err := t.store.Automations().Get(ctx, j.OriginID); err == nil && a.ProjectID == sc.ProjectID {
				return a, nil
			}
		}
	}
	if sc.ConversationID != "" {
		list, err := t.store.Automations().List(ctx, sc.ProjectID)
		if err != nil {
			return storage.Automation{}, err
		}
		for _, a := range list {
			if a.Source == "schedule" && a.Config.ConversationID == sc.ConversationID {
				return a, nil
			}
		}
	}
	return storage.Automation{}, errors.New("lượt này không thuộc lịch nào: schedule_next_run chỉ dùng trong lượt chạy của một lịch")
}

// scheduleNextRun is schedule_next_run.
func (t *Toolbox) scheduleNextRun(ctx context.Context, sc Scope, raw json.RawMessage, now time.Time) (string, error) {
	var in struct {
		Minutes int    `json:"minutes"`
		At      string `json:"at"`
		Reason  string `json:"reason"`
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", err
		}
	}
	a, err := t.scheduleOf(ctx, sc)
	if err != nil {
		return "", err
	}
	if a.Source != "schedule" || !a.Enabled {
		return "", fmt.Errorf("lịch %q đang tắt", a.Name)
	}
	var at time.Time
	switch {
	case in.Minutes > 0 && strings.TrimSpace(in.At) == "":
		at = now.Add(time.Duration(in.Minutes) * time.Minute)
	case in.Minutes == 0 && strings.TrimSpace(in.At) != "":
		if at, err = time.Parse(time.RFC3339, strings.TrimSpace(in.At)); err != nil {
			return "", errors.New("at phải dạng RFC3339, ví dụ 2026-10-26T14:30:00+07:00")
		}
	default:
		return "", errors.New("cho minutes hoặc at, một trong hai")
	}
	if d := at.Sub(now); d < MinNextRun || d > MaxNextRun {
		return "", errors.New("lượt sau phải cách bây giờ từ 10 phút tới 7 ngày")
	}
	if end := a.Config.EndsAt; end != nil && at.After(*end) {
		return "", errors.New("lịch đã dừng trước giờ đó")
	}
	at = at.UTC()
	if err := t.store.Automations().SetWake(ctx, a.ID, &at); err != nil {
		return "", err
	}
	return fmt.Sprintf("Schedule %q runs next at %s (once; its usual timing resumes after).", a.Name, at.In(time.Local).Format("15:04 02/01")), nil
}
