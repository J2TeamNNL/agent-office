package transfer

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tung1998/agent-office/internal/actor"
	"github.com/tung1998/agent-office/internal/features"
	"github.com/tung1998/agent-office/internal/perm"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/trigger"
)

// The rest of a project's setup a pack carries with its config (ADR-190):
// what a project needs to run and grow on its own once imported.

// PackAutomation is a scheduled automation; its agent is named by key.
// Webhooks and bot messages stay behind: they need a secret or a channel of
// this office.
type PackAutomation struct {
	Name         string                       `json:"name"`
	Enabled      bool                         `json:"enabled"`
	EveryMinutes int                          `json:"every_minutes,omitempty"`
	Cron         string                       `json:"cron,omitempty"`
	Timezone     string                       `json:"timezone,omitempty"`
	Action       string                       `json:"action"` // chat | workflow | script
	Workflow     string                       `json:"workflow,omitempty"`
	Agent        string                       `json:"agent,omitempty"` // "" = the default agent
	Prompt       string                       `json:"prompt,omitempty"`
	EditMode     string                       `json:"edit_mode,omitempty"`
	ModelTier    string                       `json:"model_tier,omitempty"`
	KeepContext  bool                         `json:"keep_context,omitempty"`
	Script       *storage.AutomationScript    `json:"script,omitempty"`
	Goal         *storage.AutomationGoal      `json:"goal,omitempty"`
	OnlyIf       *storage.AutomationCondition `json:"only_if,omitempty"` // ADR-194
	Tags         []string                     `json:"tags,omitempty"`
	Limits       storage.AutomationLimits     `json:"limits"`
}

// PackPolicy is the part of a project's policy that runs nothing: command
// packs and quick checks stay behind, a pack must not make commands run on
// their own.
type PackPolicy struct {
	DenyPaths     []string `json:"deny_paths,omitempty"`
	WorktreeLinks []string `json:"worktree_links,omitempty"`
	QuickCheck    *bool    `json:"quick_check,omitempty"`
}

// PackCollection is a data table's definition; Records seed a new one (an
// export leaves them out: the data is the project's own).
type PackCollection struct {
	Key         string           `json:"key"`
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Pack        string           `json:"pack,omitempty"`
	Fields      []storage.Field  `json:"fields"`
	Records     []map[string]any `json:"records,omitempty"`
}

// Collections defines data tables (collections.Service; it imports this
// package through officetools, so office hands it in).
type Collections interface {
	Define(ctx context.Context, projectID string, c storage.Collection) (storage.Collection, error)
	Upsert(ctx context.Context, projectID, key, id string, data map[string]any, by string) (storage.Record, bool, error)
}

// SetCollections lets a pack define data tables.
func (s *Service) SetCollections(c Collections) { s.collections = c }

// exportSetup puts r's automations, policy, features, data tables and Burn on p.
func (s *Service) exportSetup(ctx context.Context, r storage.Repo, keyOf map[string]string, p *Pack) error {
	autos, err := s.store.Automations().List(ctx, r.ID)
	if err != nil {
		return err
	}
	for _, a := range autos {
		if a.Source != "schedule" {
			continue
		}
		pa := PackAutomation{Name: a.Name, Enabled: a.Enabled, EveryMinutes: a.Config.EveryMinutes, Cron: a.Config.Cron, Timezone: a.Config.Timezone,
			Action: a.Action, Workflow: a.Config.Workflow, Agent: keyOf[a.AgentID], Prompt: a.Prompt, EditMode: a.EditMode, ModelTier: a.ModelTier,
			KeepContext: a.KeepContext, Goal: a.Config.Goal, OnlyIf: a.Config.OnlyIf, Tags: a.Config.Tags, Limits: a.Limits}
		if a.Action == "script" {
			sc := a.Script
			pa.Script = &sc
		}
		p.Automations = append(p.Automations, pa)
	}
	pol := perm.LoadPolicy(ctx, s.store, r.ID)
	p.Policy = &PackPolicy{DenyPaths: pol.DenyPaths, WorktreeLinks: pol.WorktreeLinks, QuickCheck: &pol.QuickCheck}
	if fc := features.LoadConfig(ctx, s.store, r.ID); len(fc.Packs) > 0 || len(fc.Off) > 0 {
		p.Features = &fc
	}
	cols, err := s.store.Collections().List(ctx, r.ID)
	if err != nil {
		return err
	}
	for _, c := range cols {
		p.Collections = append(p.Collections, PackCollection{Key: c.Key, Name: c.Name, Description: c.Description, Pack: c.Pack, Fields: c.Fields})
	}
	return s.exportBurn(ctx, r, keyOf, p)
}

// importSetup puts p's automations, policy, features and data tables into
// project r. What the project already has (an automation or a table of that
// name) is kept as it is: the project may have grown since.
func (s *Service) importSetup(ctx context.Context, p Pack, r storage.Repo, byKey map[string]storage.Agent) ([]Change, error) {
	var out []Change
	if pp := p.Policy; pp != nil {
		_, err := perm.UpdatePolicy(ctx, s.store, r.ID, func(pol *perm.Policy) (bool, error) {
			if pp.DenyPaths != nil {
				pol.DenyPaths = pp.DenyPaths
			}
			if pp.WorktreeLinks != nil {
				pol.WorktreeLinks = pp.WorktreeLinks
			}
			if pp.QuickCheck != nil {
				pol.QuickCheck = *pp.QuickCheck
			}
			return true, nil
		})
		if err != nil {
			return out, err
		}
		out = append(out, Change{Kind: "policy", Name: "quyền & lệnh", Op: "update"})
	}
	if p.Features != nil {
		if err := features.Save(ctx, s.store, r.ID, *p.Features); err != nil {
			return out, err
		}
		out = append(out, Change{Kind: "features", Name: "gói tính năng", Op: "update"})
	}
	got, err := s.importCollections(ctx, p.Collections, r)
	out = append(out, got...)
	if err != nil {
		return out, err
	}
	got, err = s.importAutomations(ctx, p.Automations, r, byKey)
	out = append(out, got...)
	if err != nil {
		return out, err
	}
	got, err = s.importBurn(ctx, p.Burn, r, byKey)
	return append(out, got...), err
}

// importCollections defines the tables r lacks, a table before those that
// point to it, and seeds the new ones with their records.
func (s *Service) importCollections(ctx context.Context, in []PackCollection, r storage.Repo) ([]Change, error) {
	var out []Change
	if len(in) == 0 {
		return out, nil
	}
	if s.collections == nil {
		return append(out, Change{Kind: "collection", Name: fmt.Sprintf("%d bảng dữ liệu", len(in)), Op: "skip", Detail: "office này không nhập bảng dữ liệu"}), nil
	}
	have, err := s.store.Collections().List(ctx, r.ID)
	if err != nil {
		return out, err
	}
	left := slices.DeleteFunc(slices.Clone(in), func(pc PackCollection) bool {
		if slices.ContainsFunc(have, func(c storage.Collection) bool { return c.Key == pc.Key }) {
			out = append(out, Change{Kind: "collection", Name: pc.Key, Op: "unchanged", Detail: "project đã có bảng này"})
			return true
		}
		return false
	})
	for len(left) > 0 { // a table pointing to one not made yet waits a round
		var wait []PackCollection
		var lastErr error
		for _, pc := range left {
			_, err := s.collections.Define(ctx, r.ID, storage.Collection{Key: pc.Key, Name: pc.Name, Description: pc.Description, Pack: pc.Pack, Fields: pc.Fields})
			if err != nil {
				wait, lastErr = append(wait, pc), err
				continue
			}
			seeded := 0
			for _, rec := range pc.Records {
				if _, _, err := s.collections.Upsert(ctx, r.ID, pc.Key, "", rec, "import"); err != nil {
					return out, fmt.Errorf("bảng %s: %w", pc.Key, err)
				}
				seeded++
			}
			out = append(out, Change{Kind: "collection", Name: pc.Key, Op: "create", Detail: fmt.Sprintf("%d bản ghi", seeded)})
		}
		if len(wait) == len(left) {
			return out, fmt.Errorf("bảng %s: %w", wait[0].Key, lastErr)
		}
		left = wait
	}
	return out, nil
}

// importAutomations adds the scheduled automations r lacks (by name). A
// script comes in off: a person reads it before it runs.
func (s *Service) importAutomations(ctx context.Context, in []PackAutomation, r storage.Repo, byKey map[string]storage.Agent) ([]Change, error) {
	var out []Change
	if len(in) == 0 {
		return out, nil
	}
	have, err := s.store.Automations().List(ctx, r.ID)
	if err != nil {
		return out, err
	}
	for _, pa := range in {
		if slices.ContainsFunc(have, func(a storage.Automation) bool { return strings.EqualFold(a.Name, strings.TrimSpace(pa.Name)) }) {
			out = append(out, Change{Kind: "automation", Name: pa.Name, Op: "unchanged", Detail: "project đã có tự động hóa cùng tên"})
			continue
		}
		spec := trigger.Spec{Name: pa.Name, Source: "schedule", EveryMinutes: pa.EveryMinutes, Cron: pa.Cron, Timezone: pa.Timezone,
			Action: pa.Action, Workflow: pa.Workflow, AgentID: byKey[pa.Agent].ID, Prompt: pa.Prompt, Goal: pa.Goal, OnlyIf: pa.OnlyIf, Tags: pa.Tags}
		if pa.Script != nil {
			spec.Script = *pa.Script
		}
		if err := spec.Check(); err != nil {
			out = append(out, Change{Kind: "automation", Name: pa.Name, Op: "skip", Detail: err.Error()})
			continue
		}
		if spec.Action == "workflow" {
			if _, err := s.store.Workflows().GetByKey(ctx, r.ID, spec.Workflow); err != nil {
				out = append(out, Change{Kind: "automation", Name: pa.Name, Op: "skip", Detail: "project chưa cài quy trình #" + spec.Workflow})
				continue
			}
		}
		a := storage.Automation{ProjectID: r.ID, Enabled: pa.Enabled, CreatedBy: importer(ctx), ModelTier: pa.ModelTier, KeepContext: pa.KeepContext,
			Limits: pa.Limits, PermissionMode: "agent"}
		if a.ModelTier != "" && !storage.ValidTier(a.ModelTier) {
			a.ModelTier = ""
		}
		if pa.EditMode == perm.EditWorktree || pa.EditMode == perm.EditDirect {
			a.EditMode = pa.EditMode
		}
		detail := ""
		if spec.Action == "script" && a.Enabled {
			a.Enabled, detail = false, "script: tắt, đọc rồi bật tay"
		}
		spec.Apply(&a, time.Now().UTC())
		if _, err := s.store.Automations().Create(ctx, a); err != nil {
			return out, err
		}
		out = append(out, Change{Kind: "automation", Name: pa.Name, Op: "create", Detail: detail})
	}
	return out, nil
}

// importer is who imports: the person behind ctx (their automations speak
// for them, ADR-194), else "import".
func importer(ctx context.Context) string {
	if who, ok := strings.CutPrefix(actor.From(ctx), "human:"); ok && who != "" {
		return who
	}
	return "import"
}
