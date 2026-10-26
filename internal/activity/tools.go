package activity

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tung1998/agent-office/internal/officetools"
	"github.com/tung1998/agent-office/internal/storage"
)

// Tools gives agents project_report (an officetools.Provider).
type Tools struct{ st storage.Store }

// NewTools is project_report over st.
func NewTools(st storage.Store) Tools { return Tools{st} }

// Tools: project_report, in any project.
func (t Tools) Tools(sc officetools.Scope) []officetools.Tool {
	if sc.ProjectID == "" || sc.Office {
		return nil
	}
	return []officetools.Tool{{Name: "project_report", ReadOnly: true,
		Description: "How this project and the projects it links to (its units) worked, in figures: each work board's open tasks by status and what was added, done or failed; each agent's runs, errors and cost; the changes to agents, workflows, schedules and tables; the schedules. For a report (a morning report) or to see where the work stands.",
		Schema: map[string]any{"type": "object", "properties": map[string]any{
			"hours": map[string]any{"type": "integer", "description": "How many hours back (default 24, at most 720)"}}}}}
}

// Call runs project_report.
func (t Tools) Call(ctx context.Context, sc officetools.Scope, name string, raw json.RawMessage) (string, bool) {
	var in struct{ Hours int }
	_ = json.Unmarshal(raw, &in)
	if in.Hours <= 0 {
		in.Hours = 24
	}
	in.Hours = min(in.Hours, 720)
	ov, err := Load(ctx, t.st, Linked(ctx, t.st, sc.ProjectID), time.Now().Add(-time.Duration(in.Hours)*time.Hour))
	if err != nil {
		return err.Error(), true
	}
	return ov.Report(), false
}
