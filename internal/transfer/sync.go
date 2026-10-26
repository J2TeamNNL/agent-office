package transfer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/tung1998/agent-office/internal/features"
	"github.com/tung1998/agent-office/internal/perm"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/team"
	"github.com/tung1998/agent-office/internal/trigger"
)

// SyncPack brings project r up to pack p, a newer version of the template it
// was made from (ADR-206). Unlike ImportPack it never replaces the project:
//   - agents by key: the template's role, instructions, tier and permissions
//     (the connection, effort, avatar and administrator rights stay the
//     project's); missing ones are made; ones the template lacks are kept;
//   - workflows by key, scheduled automations by name: updated to the
//     template, an automation keeping whether it is on (a changed script
//     comes back off, a person reads it); missing ones are made;
//   - data tables and notes the project lacks are added, feature packs the
//     template turns on are turned on; the project's data, chats, policy and
//     Burn are its own.
//
// dryRun only says what it would change.
func (s *Service) SyncPack(ctx context.Context, p Pack, r storage.Repo, dryRun bool) (Result, error) {
	res := Result{DryRun: dryRun, Changes: []Change{}}
	add := func(kind, name, op, detail string) {
		res.Changes = append(res.Changes, Change{Kind: kind, Name: name, Op: op, Detail: detail})
	}
	agents, err := s.store.Agents().List(ctx, r.ID)
	if err != nil {
		return res, err
	}
	byKey := map[string]storage.Agent{}
	for _, a := range agents {
		byKey[a.Key] = a
	}

	// agents
	var specs []team.AgentSpec
	if src := cmp(p.Project.Agents, p.Project.Model); src != nil {
		specs = src.Agents
	}
	if err := team.Validate(specs); err != nil {
		return res, err
	}
	for i, spec := range specs {
		cur, ok := byKey[spec.Key]
		if !ok {
			add("agent", spec.Name, "create", "")
			if !dryRun {
				spec.ProviderID, spec.Fallbacks = "", nil // this office's connections: the default one
				a, err := s.store.Agents().Create(ctx, spec.Agent(r.ID, len(agents)+i))
				if err != nil {
					return res, err
				}
				byKey[a.Key] = a
			}
			continue
		}
		next := cur
		next.Name, next.Role, next.Description, next.Instructions = spec.Name, spec.Role, spec.Description, spec.Instructions
		if storage.ValidTier(spec.ModelTier) {
			next.ModelTier = spec.ModelTier
		}
		perms := spec.Permissions // ADR-074: administrator rights stay as the project set them
		perms.FullAccess, perms.FullAccessBy, perms.ExtraDirs = cur.Permissions.FullAccess, cur.Permissions.FullAccessBy, cur.Permissions.ExtraDirs
		next.Permissions = perms
		if reflect.DeepEqual(next, cur) {
			add("agent", spec.Name, "unchanged", "")
			continue
		}
		add("agent", spec.Name, "update", agentDiff(cur, next))
		if !dryRun {
			if err := s.store.Agents().Update(ctx, next); err != nil {
				return res, err
			}
			byKey[next.Key] = next
		}
	}
	for _, a := range agents {
		if !slices.ContainsFunc(specs, func(x team.AgentSpec) bool { return x.Key == a.Key }) {
			add("agent", a.Name, "unchanged", "không có trong mẫu: giữ nguyên")
		}
	}

	// workflows
	var put []ProjectWorkflow
	for _, w := range p.Project.Workflows {
		cur, err := s.store.Workflows().GetByKey(ctx, r.ID, w.Key)
		switch {
		case errors.Is(err, storage.ErrNotFound):
			add("workflow", w.Key, "create", "")
			w.Enabled = true
		case err != nil:
			return res, err
		case strings.TrimSpace(cur.Source) == strings.TrimSpace(w.Source):
			add("workflow", w.Key, "unchanged", "")
			continue
		default:
			add("workflow", w.Key, "update", "")
			w.Enabled = cur.Enabled
		}
		put = append(put, w)
	}
	if bad := badWorkflow(put); bad != "" {
		return res, errors.New(bad)
	}
	if !dryRun && len(put) > 0 {
		if err := s.putWorkflows(ctx, r.ID, put); err != nil {
			return res, err
		}
	}

	// automations
	got, err := s.syncAutomations(ctx, p.Automations, r, byKey, dryRun)
	res.Changes = append(res.Changes, got...)
	if err != nil {
		return res, err
	}

	// data tables the project lacks
	if len(p.Collections) > 0 {
		have, err := s.store.Collections().List(ctx, r.ID)
		if err != nil {
			return res, err
		}
		var missing []PackCollection
		for _, pc := range p.Collections {
			if slices.ContainsFunc(have, func(c storage.Collection) bool { return c.Key == pc.Key }) {
				add("collection", pc.Key, "unchanged", "")
				continue
			}
			missing = append(missing, pc)
		}
		if dryRun {
			for _, pc := range missing {
				add("collection", pc.Key, "create", fmt.Sprintf("%d bản ghi", len(pc.Records)))
			}
		} else if len(missing) > 0 {
			got, err := s.importCollections(ctx, missing, r)
			res.Changes = append(res.Changes, got...)
			if err != nil {
				return res, err
			}
		}
	}

	// feature packs the template turns on
	if p.Features != nil {
		fc := features.LoadConfig(ctx, s.store, r.ID)
		next := features.Config{Packs: maps.Clone(fc.Packs), Off: fc.Off}
		if next.Packs == nil {
			next.Packs = map[string]bool{}
		}
		var on []string
		for k, v := range p.Features.Packs {
			if v && !next.Packs[k] {
				next.Packs[k], on = true, append(on, k)
			}
		}
		if len(on) > 0 {
			slices.Sort(on)
			add("features", "gói tính năng", "update", "bật "+strings.Join(on, ", "))
			if !dryRun {
				if err := features.Save(ctx, s.store, r.ID, next); err != nil {
					return res, err
				}
			}
		}
	}

	// notes the project's agents lack
	added := 0
	have := map[string][]storage.Memory{}
	for _, m := range p.Memories {
		a, ok := byKey[m.Agent]
		if !ok || a.ID == "" || strings.TrimSpace(m.Text) == "" {
			continue
		}
		if _, ok := have[a.ID]; !ok {
			if have[a.ID], err = s.store.Memories().List(ctx, r.ID, a.ID); err != nil {
				return res, err
			}
		}
		if slices.ContainsFunc(have[a.ID], func(x storage.Memory) bool { return x.Text == m.Text && x.Topic == m.Topic }) {
			continue
		}
		added++
		if dryRun {
			continue
		}
		nm, err := s.store.Memories().Create(ctx, storage.Memory{ProjectID: r.ID, AgentID: a.ID, Text: m.Text,
			Source: orStr(m.Source, "person"), Topic: m.Topic, Summary: m.Summary, CreatedBy: "import"})
		if err != nil {
			return res, err
		}
		have[a.ID] = append(have[a.ID], nm)
	}
	if added > 0 {
		add("memory", fmt.Sprintf("%d ghi nhớ", added), "create", "")
	}
	return res, nil
}

// agentDiff names what of an agent a sync changes.
func agentDiff(cur, next storage.Agent) string {
	var out []string
	if cur.Name != next.Name || cur.Role != next.Role || cur.Description != next.Description {
		out = append(out, "tên/vai")
	}
	if cur.Instructions != next.Instructions {
		out = append(out, "hướng dẫn")
	}
	if cur.ModelTier != next.ModelTier {
		out = append(out, "hạng model")
	}
	if !reflect.DeepEqual(cur.Permissions, next.Permissions) {
		out = append(out, "quyền")
	}
	return strings.Join(out, ", ")
}

// syncAutomations makes the template's scheduled automations the project
// lacks and updates those it has (by name) to the template, each keeping
// whether it is on. A script that changes comes back off.
func (s *Service) syncAutomations(ctx context.Context, in []PackAutomation, r storage.Repo, byKey map[string]storage.Agent, dryRun bool) ([]Change, error) {
	var out []Change
	if len(in) == 0 {
		return out, nil
	}
	have, err := s.store.Automations().List(ctx, r.ID)
	if err != nil {
		return out, err
	}
	var missing []PackAutomation
	for _, pa := range in {
		i := slices.IndexFunc(have, func(a storage.Automation) bool { return strings.EqualFold(a.Name, strings.TrimSpace(pa.Name)) })
		if i < 0 {
			missing = append(missing, pa)
			continue
		}
		cur := have[i]
		if cur.PermissionMode == "override" { // set by a person to run with other rights: theirs
			out = append(out, Change{Kind: "automation", Name: pa.Name, Op: "unchanged", Detail: "chạy với quyền riêng: giữ nguyên"})
			continue
		}
		spec := packSpec(pa, byKey)
		if err := spec.Check(); err != nil {
			out = append(out, Change{Kind: "automation", Name: pa.Name, Op: "skip", Detail: err.Error()})
			continue
		}
		next := cur
		spec.Apply(&next, time.Now().UTC())
		// the template owns when and what it runs; the rest of its setup
		// (where it reports, a run's own next time) stays the project's
		cfg, nc := cur.Config, next.Config
		cfg.EveryMinutes, cfg.Cron, cfg.Timezone, cfg.Workflow, cfg.Goal, cfg.OnlyIf = nc.EveryMinutes, nc.Cron, nc.Timezone, nc.Workflow, nc.Goal, nc.OnlyIf
		if len(pa.Tags) > 0 {
			cfg.Tags = nc.Tags
		}
		next.Config = cfg
		next.Limits, next.KeepContext = pa.Limits, pa.KeepContext
		if pa.ModelTier == "" || storage.ValidTier(pa.ModelTier) {
			next.ModelTier = pa.ModelTier
		}
		if pa.EditMode == perm.EditWorktree || pa.EditMode == perm.EditDirect {
			next.EditMode = pa.EditMode
		}
		next.NextRunAt = cur.NextRunAt // when it runs next is the project's
		if reflect.DeepEqual(next, cur) {
			out = append(out, Change{Kind: "automation", Name: pa.Name, Op: "unchanged"})
			continue
		}
		detail := ""
		if next.Action == "script" && next.Enabled && !reflect.DeepEqual(next.Script, cur.Script) {
			next.Enabled, detail = false, "script đổi: tắt, đọc rồi bật tay"
		}
		if !reflect.DeepEqual(next.Config, cur.Config) || next.Action != cur.Action {
			now := time.Now().UTC()
			next.NextRunAt = nil
			if nr, err := trigger.NextRun(next.Config, now); err == nil {
				next.NextRunAt = &nr
			}
		}
		out = append(out, Change{Kind: "automation", Name: pa.Name, Op: "update", Detail: detail})
		if !dryRun {
			if err := s.store.Automations().Update(ctx, next); err != nil {
				return out, err
			}
		}
	}
	if dryRun {
		for _, pa := range missing {
			out = append(out, Change{Kind: "automation", Name: pa.Name, Op: "create"})
		}
		return out, nil
	}
	got, err := s.importAutomations(ctx, missing, r, byKey)
	return append(out, got...), err
}

// packSpec is the automation pa as the trigger spec office checks and applies.
func packSpec(pa PackAutomation, byKey map[string]storage.Agent) trigger.Spec {
	spec := trigger.Spec{Name: pa.Name, Source: "schedule", EveryMinutes: pa.EveryMinutes, Cron: pa.Cron, Timezone: pa.Timezone,
		Action: pa.Action, Workflow: pa.Workflow, AgentID: byKey[pa.Agent].ID, Prompt: pa.Prompt, Goal: pa.Goal, OnlyIf: pa.OnlyIf, Tags: pa.Tags}
	if pa.Script != nil {
		spec.Script = *pa.Script
	}
	return spec
}
