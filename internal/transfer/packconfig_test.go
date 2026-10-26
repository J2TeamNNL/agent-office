package transfer_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tung1998/agent-office/internal/collections"
	"github.com/tung1998/agent-office/internal/features"
	"github.com/tung1998/agent-office/internal/perm"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/transfer"
)

// ADR-190: a pack carries what a project needs to run on its own (scheduled
// automations, policy, features, data tables), and a folder holding
// office.pack.json becomes that project.
func TestPackCarriesSetup(t *testing.T) {
	ctx := context.Background()
	src := newOffice(t)
	src.tr.SetCollections(collections.New(src.st))
	repo, _ := src.st.Repos().Create(ctx, storage.Repo{Name: "acme", Path: "/code/acme", Description: "Công ty"})
	if err := src.org.AddFromLibrary(ctx, repo.ID, "office/team-lead"); err != nil {
		t.Fatal(err)
	}
	agents, _ := src.st.Agents().List(ctx, repo.ID)
	ceo := agents[0]
	for _, a := range []storage.Automation{
		{ProjectID: repo.ID, Name: "Họp sáng", Enabled: true, Source: "schedule", Action: "chat", AgentID: ceo.ID, Prompt: "báo cáo",
			Config: storage.AutomationConfig{Cron: "0 8 * * 1-5", Timezone: "Asia/Ho_Chi_Minh", OnlyIf: &storage.AutomationCondition{Collection: "goals", Changed: true}}, Limits: storage.AutomationLimits{DailyCostUSD: 2}},
		{ProjectID: repo.ID, Name: "Đếm", Enabled: true, Source: "schedule", Action: "script", Config: storage.AutomationConfig{EveryMinutes: 60},
			Script: storage.AutomationScript{Lang: "bash", Body: "echo 1"}},
		{ProjectID: repo.ID, Name: "Hook", Enabled: true, Source: "webhook", Action: "chat", Prompt: "x"},
	} {
		if _, err := src.st.Automations().Create(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	perm.UpdatePolicy(ctx, src.st, repo.ID, func(p *perm.Policy) (bool, error) {
		p.DenyPaths, p.Packs = []string{"secrets/**"}, []perm.Pack{{ID: "x", Label: "x", Commands: []string{"rm -rf *"}}}
		return true, nil
	})
	features.Save(ctx, src.st, repo.ID, features.Config{Packs: map[string]bool{"develop": false}})
	if _, err := collections.New(src.st).Define(ctx, repo.ID, storage.Collection{Key: "goals", Name: "Mục tiêu",
		Fields: []storage.Field{{Name: "title", Type: storage.FieldText}, {Name: "target", Type: storage.FieldNumber}}}); err != nil {
		t.Fatal(err)
	}
	src.st.Memories().Create(ctx, storage.Memory{ProjectID: repo.ID, AgentID: ceo.ID, Text: "khách VIP ưu tiên", Source: "agent"})

	pack, err := src.tr.ExportPack(ctx, repo.ID, []string{"config", "memory"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Automations) != 2 || pack.Automations[0].Agent != ceo.Key || pack.Policy == nil || pack.Features == nil || len(pack.Collections) != 1 {
		t.Fatalf("pack: automations %+v policy %+v features %+v collections %+v", pack.Automations, pack.Policy, pack.Features, pack.Collections)
	}
	// a table pointing to one listed after it, with seed records
	pack.Collections = append([]transfer.PackCollection{{Key: "kpis", Name: "KPI", Fields: []storage.Field{
		{Name: "name", Type: storage.FieldText}, {Name: "goal", Type: storage.FieldRef, Ref: "goals"}},
		Records: []map[string]any{{"name": "Doanh thu"}}}}, pack.Collections...)
	pack.Collections[1].Records = []map[string]any{{"title": "Kiếm 1000$", "target": 1000}}
	raw, _ := json.Marshal(pack)
	folder := t.TempDir()
	os.WriteFile(filepath.Join(folder, transfer.RepoPackFile), raw, 0o644)

	dst := newOffice(t)
	dst.tr.SetCollections(collections.New(dst.st))
	np := storage.NewProject{Path: folder}
	if err := dst.tr.CheckNewProject(ctx, &np); err != nil || np.Name != "acme" {
		t.Fatalf("check: %+v %v", np, err)
	}
	got, err := dst.tr.CreateProject(ctx, np)
	if err != nil {
		t.Fatal(err)
	}
	autos, _ := dst.st.Automations().List(ctx, got.ID)
	if len(autos) != 2 {
		t.Fatalf("automations: %+v", autos)
	}
	gotAgents, _ := dst.st.Agents().List(ctx, got.ID)
	for _, a := range autos {
		switch a.Name {
		case "Họp sáng":
			if !a.Enabled || a.AgentID != gotAgents[0].ID || a.NextRunAt == nil || a.Limits.DailyCostUSD != 2 || a.CreatedBy != "import" || a.Config.OnlyIf == nil || !a.Config.OnlyIf.Changed {
				t.Fatalf("chat automation: %+v", a)
			}
		case "Đếm":
			if a.Enabled {
				t.Fatal("a script came in on: a person reads it first")
			}
		}
	}
	pol := perm.LoadPolicy(ctx, dst.st, got.ID)
	if !slices.Equal(pol.DenyPaths, []string{"secrets/**"}) || len(pol.Packs) != 0 {
		t.Fatalf("policy: deny %v packs %v (commands must not come with a pack)", pol.DenyPaths, pol.Packs)
	}
	if fc := features.LoadConfig(ctx, dst.st, got.ID); fc.Packs["develop"] {
		t.Fatalf("features: %+v", fc)
	}
	for key, n := range map[string]int{"goals": 1, "kpis": 1} {
		if _, total, err := collections.New(dst.st).Query(ctx, got.ID, key, collections.Query{}); err != nil || total != n {
			t.Fatalf("%s: %d records, %v", key, total, err)
		}
	}
	if notes, _ := dst.st.Memories().List(ctx, got.ID, gotAgents[0].ID); len(notes) != 1 {
		t.Fatalf("notes: %+v", notes)
	}
	// again: what the project has (grown since) stays as it is
	if _, err := dst.tr.ImportPack(ctx, pack, got, []string{"config"}); err != nil {
		t.Fatal(err)
	}
	if autos, _ := dst.st.Automations().List(ctx, got.ID); len(autos) != 2 {
		t.Fatalf("automations after a second import: %d", len(autos))
	}
	if _, total, _ := collections.New(dst.st).Query(ctx, got.ID, "goals", collections.Query{}); total != 1 {
		t.Fatalf("goals after a second import: %d", total)
	}
}

// A pack whose config does not take (a workflow that does not read) makes no
// project: no tables or schedules without the agents they were made for.
func TestBadPackMakesNothing(t *testing.T) {
	ctx := context.Background()
	o := newOffice(t)
	o.tr.SetCollections(collections.New(o.st))
	folder := t.TempDir()
	pack := `{"version":2,"include":["config"],"project":{"name":"acme","agents":{"agents":[{"key":"ceo","name":"CEO","model_tier":"strong"}]},
	  "workflows":[{"key":"x","source":"---\nkey: x\nname: X\nroles: []\nbrief: [nope]\n---\nbody","enabled":true}]},
	  "collections":[{"key":"goals","fields":[{"name":"title","type":"text"}]}]}`
	os.WriteFile(filepath.Join(folder, transfer.RepoPackFile), []byte(pack), 0o644)
	if _, err := o.tr.CreateProject(ctx, storage.NewProject{Path: folder}); err == nil {
		t.Fatal("a pack that does not take made a project")
	}
	if list, _ := o.st.Repos().List(ctx); len(list) != 0 {
		t.Fatalf("projects left: %d", len(list))
	}
}
