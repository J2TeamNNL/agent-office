package transfer_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tung1998/agent-office/internal/burn"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/transfer"
)

// ADR-196: a pack carries the work board: the Burn's settings, its review
// profiles (reviewers by key), what it learned, and seed quests. It comes in
// off with its own schedule off; a second import adds no quest twice and
// keeps the settings the project has.
func TestPackCarriesBurn(t *testing.T) {
	ctx := context.Background()
	src := newOffice(t)
	src.tr.SetBurnCheck(burn.CheckSetup)
	repo, _ := src.st.Repos().Create(ctx, storage.Repo{Name: "acme", Path: "/code/acme"})
	if err := src.org.AddFromLibrary(ctx, repo.ID, "office/team-lead", "office/code-reviewer"); err != nil {
		t.Fatal(err)
	}
	agents, _ := src.st.Agents().List(ctx, repo.ID)
	lead, reviewer := agents[0], agents[1]
	rp, err := src.st.Burn().SaveReviewProfile(ctx, storage.BurnReviewProfile{ProjectID: repo.ID, Name: "Chặt",
		Stages: map[string]storage.BurnReviewStage{"result": {AgentID: reviewer.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := src.st.Burn().SaveSession(ctx, storage.BurnSession{ProjectID: repo.ID, AgentID: lead.ID, ModelTier: storage.TierStrong, MaxParallel: 2,
		Order: "roadmap", Template: "ux", Verify: "go test ./...", Lessons: "- nhỏ thôi", ReviewProfileID: rp.ID, State: "running",
		Plan: storage.BurnPlan{End: "hours", Hours: 8, Auto: true, Days: []int{1, 2, 3, 4, 5}, At: "08:30", TZ: "Asia/Ho_Chi_Minh"}})
	if err != nil {
		t.Fatal(err)
	}
	src.st.Burn().AddItem(ctx, storage.BurnItem{SessionID: b.ID, Title: "việc riêng", Kind: "quest", Detail: "x", Status: "found"})

	pack, err := src.tr.ExportPack(ctx, repo.ID, []string{"config"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pb := pack.Burn
	if pb == nil || pb.Agent != lead.Key || pb.Template != "ux" || pb.Review != "Chặt" || len(pb.ReviewProfiles) != 1 ||
		pb.ReviewProfiles[0].Stages["result"].Agent != reviewer.Key || !pb.Plan.Auto || pb.Lessons == "" || len(pb.Quests) != 0 {
		t.Fatalf("exported burn: %+v", pb)
	}
	pb.Quests = []transfer.PackQuest{{Title: "Viết hồ sơ công ty", Detail: "docs/company.md"}, {Title: "Không mô tả"}}
	raw, _ := json.Marshal(pack)
	var back transfer.Pack
	json.Unmarshal(raw, &back)

	dst := newOffice(t)
	dst.tr.SetBurnCheck(burn.CheckSetup)
	got, _ := dst.st.Repos().Create(ctx, storage.Repo{Name: "acme", Path: t.TempDir()})
	res, err := dst.tr.ImportPack(ctx, back, got, []string{"config"})
	if err != nil {
		t.Fatal(err)
	}
	nb, err := dst.st.Burn().Session(ctx, got.ID)
	if err != nil {
		t.Fatalf("no Burn: %v %+v", err, res.Changes)
	}
	gotAgents, _ := dst.st.Agents().List(ctx, got.ID)
	keyOf := map[string]string{}
	for _, a := range gotAgents {
		keyOf[a.ID] = a.Key
	}
	if nb.Active() || nb.Plan.Auto || nb.Plan.At != "08:30" || keyOf[nb.AgentID] != lead.Key || nb.Template != "ux" || nb.MaxParallel != 2 || nb.Verify != "go test ./..." {
		t.Fatalf("imported burn (must come in off, its schedule off): %+v", nb)
	}
	prof, err := dst.st.Burn().ReviewProfile(ctx, nb.ReviewProfileID)
	if err != nil || keyOf[prof.Stages["result"].AgentID] != reviewer.Key {
		t.Fatalf("review profile: %+v %v", prof, err)
	}
	items, _ := dst.st.Burn().Items(ctx, nb.ID)
	if len(items) != 1 || items[0].Kind != "quest" || items[0].Status != "found" {
		t.Fatalf("quests (one without detail is left out): %+v", items)
	}
	// again: the project's own settings stay, no quest twice
	nb.Template = "test"
	dst.st.Burn().SaveSession(ctx, nb)
	if _, err := dst.tr.ImportPack(ctx, back, got, []string{"config"}); err != nil {
		t.Fatal(err)
	}
	nb, _ = dst.st.Burn().Session(ctx, got.ID)
	items, _ = dst.st.Burn().Items(ctx, nb.ID)
	profiles, _ := dst.st.Burn().ReviewProfiles(ctx, got.ID)
	if nb.Template != "test" || len(items) != 1 || len(profiles) != 1 {
		t.Fatalf("second import: template %s, %d quests, %d profiles", nb.Template, len(items), len(profiles))
	}
	// a Burn that could not run is not set up
	bad := newOffice(t)
	bad.tr.SetBurnCheck(burn.CheckSetup)
	other, _ := bad.st.Repos().Create(ctx, storage.Repo{Name: "x", Path: t.TempDir()})
	back.Burn.Template = "nope"
	if _, err := bad.tr.ImportPack(ctx, back, other, []string{"config"}); err != nil {
		t.Fatal(err)
	}
	if _, err := bad.st.Burn().Session(ctx, other.ID); err == nil {
		t.Fatal("a Burn with an unknown template was set up")
	}
}
