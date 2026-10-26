package activity_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tung1998/agent-office/internal/activity"
	"github.com/tung1998/agent-office/internal/officetools"
	"github.com/tung1998/agent-office/internal/perm"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/storage/sqlite"
)

// ADR-210: a project and those it links to, as one: their work boards,
// agents' runs and cost; a project it does not link to is left out.
func TestActivity(t *testing.T) {
	ctx := context.Background()
	st, _ := sqlite.Open(filepath.Join(t.TempDir(), "o.db"))
	defer st.Close()
	st.Migrate(ctx)
	mk := func(name string) storage.Repo {
		r, _ := st.Repos().Create(ctx, storage.Repo{Name: name, Path: t.TempDir()})
		return r
	}
	hq, unit, other := mk("HQ"), mk("Theme Studio"), mk("Khác")
	perm.UpdatePolicy(ctx, st, hq.ID, func(p *perm.Policy) (bool, error) {
		p.LinkedProjects = append(p.LinkedProjects, unit.ID)
		return true, nil
	})
	if ps := activity.Linked(ctx, st, hq.ID); len(ps) != 2 || ps[1].ID != unit.ID {
		t.Fatalf("linked: %+v", ps)
	}
	dev, _ := st.Agents().Create(ctx, storage.Agent{ProjectID: unit.ID, Key: "thuc-thi", Name: "Thực thi", ModelTier: storage.TierBalanced})
	b, _ := st.Burn().SaveSession(ctx, storage.BurnSession{ProjectID: unit.ID})
	st.Burn().AddItem(ctx, storage.BurnItem{SessionID: b.ID, Title: "a", Kind: "quest", Status: "handed"})
	st.Burn().AddItem(ctx, storage.BurnItem{SessionID: b.ID, Title: "b", Kind: "quest", Status: "done"})
	cost := 0.5
	st.Runs().Create(ctx, storage.Run{Kind: "chat", ProjectID: unit.ID, AgentID: dev.ID, Status: "ok", CostUSD: &cost})
	st.Runs().Create(ctx, storage.Run{Kind: "chat", ProjectID: other.ID, Status: "ok", CostUSD: &cost})
	ov, err := activity.Load(ctx, st, activity.Linked(ctx, st, hq.ID), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if ov.Runs != 1 || ov.CostUSD != 0.5 || len(ov.Projects) != 2 || ov.Projects[1].ByStatus["handed"] != 1 || ov.Projects[1].Done != 1 {
		t.Fatalf("overview: %+v", ov)
	}
	if r := ov.Report(); !strings.Contains(r, "Theme Studio / Thực thi: 1 runs") || strings.Contains(r, "Khác") {
		t.Fatalf("report:\n%s", r)
	}
	tools := activity.NewTools(st)
	if len(tools.Tools(officetools.Scope{ProjectID: hq.ID})) != 1 || len(tools.Tools(officetools.Scope{ProjectID: hq.ID, Office: true})) != 0 {
		t.Fatal("project_report missing, or given to the office assistant")
	}
	if out, isErr := tools.Call(ctx, officetools.Scope{ProjectID: hq.ID}, "project_report", nil); isErr || !strings.Contains(out, "Theme Studio") {
		t.Fatalf("tool: %s", out)
	}
}
