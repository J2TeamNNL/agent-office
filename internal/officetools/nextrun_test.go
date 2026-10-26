package officetools

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/tung1998/agent-office/internal/perm"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/storage/sqlite"
)

// ADR-202: a run of a schedule (its job, or the chat it keeps, where a
// delegate's answer calls it again) sets the schedule's next run once,
// between 10 minutes and 7 days; any other chat has no such tool.
func TestScheduleNextRun(t *testing.T) {
	ctx := context.Background()
	st, _ := sqlite.Open(filepath.Join(t.TempDir(), "o.db"))
	defer st.Close()
	st.Migrate(ctx)
	p, _ := st.Repos().Create(ctx, storage.Repo{Name: "hq", Path: t.TempDir()})
	conv, _ := st.Chat().CreateConversation(ctx, storage.Conversation{ProjectID: p.ID, Title: "điều phối"})
	other, _ := st.Chat().CreateConversation(ctx, storage.Conversation{ProjectID: p.ID, Title: "khác"})
	a, _ := st.Automations().Create(ctx, storage.Automation{ProjectID: p.ID, Name: "Điều phối", Source: "schedule", Action: "chat", Enabled: true, Prompt: "x",
		Config: storage.AutomationConfig{EveryMinutes: 60, ConversationID: conv.ID}})
	tb := New(st, nil, nil)
	run := Scope{ProjectID: p.ID, ConversationID: conv.ID, Agent: "CEO", Level: perm.Operate}
	chat := run
	chat.ConversationID = other.ID
	has := func(sc Scope) bool {
		return slices.ContainsFunc(tb.ToolsFor(sc), func(x Tool) bool { return x.Name == "schedule_next_run" })
	}
	if !has(run) || has(chat) {
		t.Fatal("the tool is for a schedule's runs only")
	}
	now := time.Now()
	for _, bad := range []string{`{"minutes":5,"reason":"r"}`, `{"minutes":20000,"reason":"r"}`, `{"reason":"r"}`, `{"minutes":30,"at":"2026-01-01T00:00:00Z","reason":"r"}`} {
		if _, err := tb.scheduleNextRun(ctx, run, []byte(bad), now); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	if _, err := tb.scheduleNextRun(ctx, chat, []byte(`{"minutes":20,"reason":"r"}`), now); err == nil {
		t.Fatal("a chat no schedule keeps set a schedule's next run")
	}
	if _, err := tb.scheduleNextRun(ctx, run, []byte(`{"minutes":20,"reason":"việc giao xong trong 20 phút"}`), now); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Automations().Get(ctx, a.ID)
	if got.Config.WakeAt == nil || got.NextRunAt == nil || got.NextRunAt.Sub(now).Round(time.Minute) != 20*time.Minute {
		t.Fatalf("next run: %v wake %v", got.NextRunAt, got.Config.WakeAt)
	}
	if got.Config.EveryMinutes != 60 {
		t.Fatal("its own timing changed")
	}
}
