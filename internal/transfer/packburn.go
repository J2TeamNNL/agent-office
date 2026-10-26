package transfer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tung1998/agent-office/internal/storage"
)

// A pack carries the project's work board too (ADR-196): how its Burn runs,
// who does and who reviews the work, what it has learned, and the quests a
// new project starts with. The Burn comes in off, and so does its own
// schedule: it runs with full access (ADR-074), a person turns it on.

// PackBurn is a project's Burn setup; agents are named by key.
type PackBurn struct {
	Agent       string           `json:"agent,omitempty"` // "" = the default agent
	ModelTier   string           `json:"model_tier,omitempty"`
	MaxParallel int              `json:"max_parallel,omitempty"`
	Focus       string           `json:"focus,omitempty"`
	Order       string           `json:"order,omitempty"`    // roadmap | bugs | auto
	Template    string           `json:"template,omitempty"` // ADR-128
	HuntPrompt  string           `json:"hunt_prompt,omitempty"`
	Verify      string           `json:"verify,omitempty"` // commands run on a piece reported done, one a line
	Lessons     string           `json:"lessons,omitempty"`
	ReviewCap   int              `json:"review_cap,omitempty"`
	StopAfter   int              `json:"stop_after,omitempty"`
	Plan        storage.BurnPlan `json:"plan"`
	// Review: the name of the review profile it follows ("" = none).
	Review         string              `json:"review,omitempty"`
	ReviewProfiles []PackReviewProfile `json:"review_profiles,omitempty"`
	// Quests seed the board (an export leaves them out: the work is the
	// project's own); an open quest with the same title is not added twice.
	Quests []PackQuest `json:"quests,omitempty"`
}

// PackReviewProfile is a review profile (ADR-113): each stage's reviewer by
// agent key ("" = the Burn's) and the workflow it runs.
type PackReviewProfile struct {
	Name   string                     `json:"name"`
	Stages map[string]PackReviewStage `json:"stages"`
}

// PackReviewStage is who reviews a stage.
type PackReviewStage struct {
	Agent    string `json:"agent,omitempty"`
	Workflow string `json:"workflow,omitempty"`
}

// PackQuest is a quest on the board.
type PackQuest struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Owner  string `json:"owner,omitempty"` // the agent key that follows it up (ADR-197); "" = the default agent
}

// reviewStages are the stages a profile may review.
var reviewStages = []string{"issue", "plan", "result"}

// SetBurnCheck lets a pack set up a Burn: check refuses settings a Burn could
// not run on (burn.CheckSetup; burn imports this package, so office hands it in).
func (s *Service) SetBurnCheck(check func(storage.BurnSession) error) { s.burnCheck = check }

// exportBurn puts r's Burn setup on p, when it has one.
func (s *Service) exportBurn(ctx context.Context, r storage.Repo, keyOf map[string]string, p *Pack) error {
	b, err := s.store.Burn().Session(ctx, r.ID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	pb := PackBurn{Agent: keyOf[b.AgentID], ModelTier: b.ModelTier, MaxParallel: b.MaxParallel, Focus: b.Focus, Order: b.Order,
		Template: b.Template, HuntPrompt: b.HuntPrompt, Verify: b.Verify, Lessons: b.Lessons, ReviewCap: b.ReviewCap, StopAfter: b.StopAfter, Plan: b.Plan}
	profiles, err := s.store.Burn().ReviewProfiles(ctx, r.ID)
	if err != nil {
		return err
	}
	for _, rp := range profiles {
		pp := PackReviewProfile{Name: rp.Name, Stages: map[string]PackReviewStage{}}
		for st, x := range rp.Stages {
			pp.Stages[st] = PackReviewStage{Agent: keyOf[x.AgentID], Workflow: x.Workflow}
		}
		pb.ReviewProfiles = append(pb.ReviewProfiles, pp)
		if rp.ID == b.ReviewProfileID {
			pb.Review = rp.Name
		}
	}
	p.Burn = &pb
	return nil
}

// importBurn sets up r's Burn from pb: settings only when r has none saved
// yet (it may have grown since), the review profiles it lacks (by name), and
// the quests not open on its board.
func (s *Service) importBurn(ctx context.Context, pb *PackBurn, r storage.Repo, byKey map[string]storage.Agent) ([]Change, error) {
	var out []Change
	if pb == nil {
		return out, nil
	}
	if s.burnCheck == nil {
		return append(out, Change{Kind: "burn", Name: "Burn", Op: "skip", Detail: "office này không nhập Burn"}), nil
	}
	agentOf := func(key string) string {
		if a, ok := byKey[key]; ok && !a.Disabled {
			return a.ID
		}
		return ""
	}
	have, err := s.store.Burn().ReviewProfiles(ctx, r.ID)
	if err != nil {
		return out, err
	}
	profileID := map[string]string{}
	for _, rp := range have {
		profileID[strings.ToLower(rp.Name)] = rp.ID
	}
	for _, pp := range pb.ReviewProfiles {
		name := strings.TrimSpace(pp.Name)
		if name == "" {
			continue
		}
		if _, ok := profileID[strings.ToLower(name)]; ok {
			out = append(out, Change{Kind: "burn", Name: "review: " + name, Op: "unchanged", Detail: "project đã có hồ sơ review này"})
			continue
		}
		stages := map[string]storage.BurnReviewStage{}
		for st, x := range pp.Stages {
			if !slices.Contains(reviewStages, st) {
				continue
			}
			if x.Workflow != "" {
				if _, err := s.store.Workflows().GetByKey(ctx, r.ID, x.Workflow); err != nil {
					x.Workflow = "" // its reviewer answers itself
				}
			}
			stages[st] = storage.BurnReviewStage{AgentID: agentOf(x.Agent), Workflow: x.Workflow}
		}
		saved, err := s.store.Burn().SaveReviewProfile(ctx, storage.BurnReviewProfile{ProjectID: r.ID, Name: name, Stages: stages})
		if err != nil {
			return out, err
		}
		profileID[strings.ToLower(name)] = saved.ID
		out = append(out, Change{Kind: "burn", Name: "review: " + name, Op: "create"})
	}

	b, err := s.store.Burn().Session(ctx, r.ID)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		b = storage.BurnSession{ProjectID: r.ID, ModelTier: storage.TierBalanced, MaxParallel: 1, Order: "roadmap", State: "stopped",
			AgentID: agentOf(pb.Agent), Focus: strings.TrimSpace(pb.Focus), Template: pb.Template, HuntPrompt: strings.TrimSpace(pb.HuntPrompt),
			Verify: strings.TrimSpace(pb.Verify), Lessons: pb.Lessons, ReviewCap: min(max(pb.ReviewCap, 0), 100), StopAfter: min(max(pb.StopAfter, 0), 100),
			Plan: pb.Plan, ReviewProfileID: profileID[strings.ToLower(strings.TrimSpace(pb.Review))]}
		if storage.ValidTier(pb.ModelTier) {
			b.ModelTier = pb.ModelTier
		}
		if pb.MaxParallel > 0 {
			b.MaxParallel = min(pb.MaxParallel, 5)
		}
		if pb.Order != "" {
			b.Order = pb.Order
		}
		if b.AgentID == "" {
			if rr, err := s.store.Repos().Get(ctx, r.ID); err == nil { // its default agent is set by the import
				r = rr
			}
			if a, ok := storage.DefaultAgent(r, mapValues(byKey)); ok {
				b.AgentID = a.ID
			}
		}
		if err := s.burnCheck(b); err != nil {
			return append(out, Change{Kind: "burn", Name: "Burn", Op: "skip", Detail: err.Error()}), nil
		}
		var notes []string
		if b.Plan.Auto {
			b.Plan.Auto = false
			notes = append(notes, fmt.Sprintf("lịch tự bật %s: tắt, bật tay trong Cài đặt Burn", b.Plan.At))
		}
		if b.Verify != "" {
			notes = append(notes, "lệnh kiểm chứng: "+strings.ReplaceAll(b.Verify, "\n", "; "))
		}
		if b, err = s.store.Burn().SaveSession(ctx, b); err != nil {
			return out, err
		}
		out = append(out, Change{Kind: "burn", Name: "Burn", Op: "create", Detail: strings.Join(notes, " · ")})
	case err != nil:
		return out, err
	default:
		out = append(out, Change{Kind: "burn", Name: "Burn", Op: "unchanged", Detail: "project đã có cài đặt Burn"})
	}

	if len(pb.Quests) == 0 {
		return out, nil
	}
	items, err := s.store.Burn().Items(ctx, b.ID)
	if err != nil {
		return out, err
	}
	open := map[string]bool{}
	for _, it := range items {
		if it.Status != "done" && it.Status != "failed" && it.Status != "skipped" {
			open[questKey(it.Title)] = true
		}
	}
	added := 0
	for _, q := range pb.Quests {
		title, detail := strings.Join(strings.Fields(q.Title), " "), strings.TrimSpace(q.Detail)
		if title == "" || detail == "" || len([]rune(title)) > 160 || len([]rune(detail)) > 4000 || open[questKey(title)] {
			continue
		}
		if _, err := s.store.Burn().AddItem(ctx, storage.BurnItem{SessionID: b.ID, Title: title, Kind: "quest", Detail: detail, Status: "found", Owner: ownerKey(q.Owner, byKey)}); err != nil {
			return out, err
		}
		open[questKey(title)] = true
		added++
	}
	return append(out, Change{Kind: "burn", Name: "quest", Op: "create", Detail: fmt.Sprintf("%d quest", added)}), nil
}

// ownerKey is key when the project has that agent, else "" (its default agent).
func ownerKey(key string, byKey map[string]storage.Agent) string {
	if _, ok := byKey[key]; ok {
		return key
	}
	return ""
}

// questKey is a title as the board compares it: case and spaces aside.
func questKey(title string) string { return strings.ToLower(strings.Join(strings.Fields(title), " ")) }

func mapValues(m map[string]storage.Agent) []storage.Agent {
	out := make([]storage.Agent, 0, len(m))
	for _, a := range m {
		out = append(out, a)
	}
	return out
}
