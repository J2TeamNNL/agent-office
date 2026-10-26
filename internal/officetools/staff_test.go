package officetools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/storage/sqlite"
)

// ADR-197: staff_load shows what each agent carried: turns, failures, cost
// and its share, schedules, notes, the quests it follows up.
func TestStaffLoad(t *testing.T) {
	ctx := context.Background()
	st, _ := sqlite.Open(filepath.Join(t.TempDir(), "o.db"))
	defer st.Close()
	st.Migrate(ctx)
	p, _ := st.Repos().Create(ctx, storage.Repo{Name: "hq", Path: t.TempDir()})
	ceo, _ := st.Agents().Create(ctx, storage.Agent{ProjectID: p.ID, Key: "ceo", Name: "Tổng giám đốc", ModelTier: storage.TierStrong})
	st.Agents().Create(ctx, storage.Agent{ProjectID: p.ID, Key: "cto", Name: "Giám đốc kỹ thuật", ModelTier: storage.TierStrong})
	for i, status := range []string{"done", "done", "failed"} {
		j, err := st.Jobs().Create(ctx, storage.Job{ProjectID: p.ID, Kind: "chat_turn", Origin: "user", AgentID: ceo.ID, Status: status, CostUSD: float64(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
		j.CostUSD = float64(i + 1)
		st.Jobs().Update(ctx, j)
	}
	st.Automations().Create(ctx, storage.Automation{ProjectID: p.ID, Name: "Điều phối", Enabled: true, Source: "schedule", Action: "chat", AgentID: ceo.ID, Prompt: "x",
		Config: storage.AutomationConfig{EveryMinutes: 60}})
	st.Memories().Create(ctx, storage.Memory{ProjectID: p.ID, AgentID: ceo.ID, Text: "Quy tắc cốt lõi", Source: "agent"})
	st.Memories().Create(ctx, storage.Memory{ProjectID: p.ID, AgentID: ceo.ID, Text: "x", Topic: "ban-hang", Source: "agent"})
	b, _ := st.Burn().SaveSession(ctx, storage.BurnSession{ProjectID: p.ID, ModelTier: storage.TierBalanced, MaxParallel: 1, State: "stopped"})
	st.Burn().AddItem(ctx, storage.BurnItem{SessionID: b.ID, Title: "q", Kind: "quest", Status: "found", Owner: "cto"})

	col, err := st.Collections().Create(ctx, storage.Collection{ProjectID: p.ID, Key: "tasks", Fields: []storage.Field{{Name: "title", Type: storage.FieldText}, {Name: "assignee", Type: storage.FieldText}, {Name: "status", Type: storage.FieldText}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []map[string]any{{"title": "a", "assignee": "cto", "status": "dang-lam"}, {"title": "b", "assignee": "Giám đốc kỹ thuật", "status": "cho"}, {"title": "c", "assignee": "cto", "status": "xong"}} {
		if _, err := st.Collections().Insert(ctx, storage.Record{CollectionID: col.ID, ProjectID: p.ID, Data: d}); err != nil {
			t.Fatal(err)
		}
	}
	tb := New(st, nil, nil)
	out, err := tb.staffLoad(ctx, Scope{ProjectID: p.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		for _, k := range []string{"(ceo)", "(cto)"} {
			if strings.Contains(l, k) {
				rows[k] = l
			}
		}
	}
	if c := rows["(ceo)"]; !strings.Contains(c, "| 3 | 1 |") || !strings.Contains(c, "(100%)") || !strings.Contains(c, "| 1 | 15 | 1 | 0 | 0 | 0 |") {
		t.Fatalf("ceo row: %q\n%s", c, out)
	}
	if c := rows["(cto)"]; !strings.Contains(c, "| 0 | 0 | 0.00 (0%) | 0 | 0 | 0 | 0 | 1 | 2 |") {
		t.Fatalf("cto row: %q", c)
	}
}
