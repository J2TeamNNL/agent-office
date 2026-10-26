// Package activity is how a set of projects worked over a time (ADR-210):
// their work boards, their agents' runs and cost, the changes to their setup
// (agents, workflows, schedules, tables) and their schedules. It reads
// office's own data and changes nothing: the overview page shows it for every
// project, an agent reads it for its project and those it links to.
package activity

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tung1998/agent-office/internal/perm"
	"github.com/tung1998/agent-office/internal/storage"
)

// Linked is a project and the projects it links to, step by step (the units
// it opened, theirs…), at most 50.
func Linked(ctx context.Context, st storage.Store, projectID string) []storage.Repo {
	var out []storage.Repo
	seen := map[string]bool{}
	next := []string{projectID}
	for len(next) > 0 && len(out) < 50 {
		id := next[0]
		next = next[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		r, err := st.Repos().Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, r)
		next = append(next, perm.LoadPolicy(ctx, st, id).LinkedProjects...)
	}
	return out
}

// Overview is how a set of projects worked since a time.
type Overview struct {
	Since     time.Time     `json:"since"`
	CostUSD   float64       `json:"cost_usd"`
	Runs      int           `json:"runs"`
	Projects  []ProjectStat `json:"projects"`
	Agents    []AgentStat   `json:"agents"`
	Changes   []Change      `json:"changes"`
	Schedules []Schedule    `json:"schedules"`
}

// ProjectStat is one project's work board: tasks by status now, and what moved.
type ProjectStat struct {
	ProjectID string         `json:"project_id"`
	Project   string         `json:"project"`
	ByStatus  map[string]int `json:"by_status"`
	Added     int            `json:"added"`
	Done      int            `json:"done"`
	Failed    int            `json:"failed"`
	CostUSD   float64        `json:"cost_usd"`
}

// AgentStat is one agent's runs since.
type AgentStat struct {
	ProjectID string  `json:"project_id"`
	Project   string  `json:"project"`
	Agent     string  `json:"agent"`
	Runs      int     `json:"runs"`
	Errors    int     `json:"errors"`
	CostUSD   float64 `json:"cost_usd"`
}

// Change is a change to a project's setup (agents, workflows, schedules, tables).
type Change struct {
	At      time.Time `json:"at"`
	Project string    `json:"project"`
	Who     string    `json:"who"`
	Action  string    `json:"action"`
	Target  string    `json:"target"`
}

// Schedule is one of the projects' schedules.
type Schedule struct {
	ProjectID string     `json:"project_id"`
	Project   string     `json:"project"`
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Enabled   bool       `json:"enabled"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
}

// setup are the change log's resources a change of a project's setup touches.
var setup = []string{"agent", "workflow", "automation", "collection", "project", "policy", "burn"}

// Load works out how projects worked since a time.
func Load(ctx context.Context, st storage.Store, projects []storage.Repo, since time.Time) (Overview, error) {
	ov := Overview{Since: since, Projects: []ProjectStat{}, Agents: []AgentStat{}, Changes: []Change{}, Schedules: []Schedule{}}
	in := map[string]bool{}
	agentOf := map[string]AgentStat{}
	var order []string
	for _, p := range projects {
		in[p.ID] = true
		agents, err := st.Agents().List(ctx, p.ID)
		if err != nil {
			return ov, err
		}
		for _, a := range agents {
			agentOf[a.ID] = AgentStat{ProjectID: p.ID, Project: p.Name, Agent: a.Name}
			order = append(order, a.ID)
		}
	}
	runs, err := st.Runs().Since(ctx, since)
	if err != nil {
		return ov, err
	}
	projCost := map[string]float64{}
	for _, r := range runs {
		if !in[r.ProjectID] {
			continue
		}
		c := 0.0
		if r.CostUSD != nil {
			c = *r.CostUSD
		}
		ov.Runs++
		ov.CostUSD += c
		projCost[r.ProjectID] += c
		if s, ok := agentOf[r.AgentID]; ok {
			s.Runs++
			s.CostUSD += c
			if r.Status == "error" {
				s.Errors++
			}
			agentOf[r.AgentID] = s
		}
	}
	for _, id := range order {
		if s := agentOf[id]; s.Runs > 0 {
			ov.Agents = append(ov.Agents, s)
		}
	}
	slices.SortStableFunc(ov.Agents, func(a, b AgentStat) int { return int((b.CostUSD - a.CostUSD) * 1e6) })
	for _, p := range projects {
		ps := ProjectStat{ProjectID: p.ID, Project: p.Name, ByStatus: map[string]int{}, CostUSD: projCost[p.ID]}
		if b, err := st.Burn().Session(ctx, p.ID); err == nil {
			items, err := st.Burn().Items(ctx, b.ID)
			if err != nil {
				return ov, err
			}
			for _, it := range items {
				if it.Kind == "" { // a scan, not a task
					continue
				}
				ps.ByStatus[it.Status]++
				if it.CreatedAt.After(since) {
					ps.Added++
				}
				if it.UpdatedAt.After(since) && it.Status == "done" {
					ps.Done++
				}
				if it.UpdatedAt.After(since) && it.Status == "failed" {
					ps.Failed++
				}
			}
		}
		ov.Projects = append(ov.Projects, ps)
		entries, err := st.Audit().List(ctx, storage.AuditFilter{ProjectID: p.ID, From: since, Limit: 200})
		if err != nil {
			return ov, err
		}
		for _, e := range entries {
			if !slices.Contains(setup, e.Resource) || strings.HasPrefix(e.Action, "data.") {
				continue
			}
			who := e.ActorName
			if who == "" {
				who = strings.TrimPrefix(e.Actor, "human:")
			}
			ov.Changes = append(ov.Changes, Change{At: e.At, Project: p.Name, Who: who, Action: e.Action, Target: e.Target})
		}
		autos, err := st.Automations().List(ctx, p.ID)
		if err != nil {
			return ov, err
		}
		for _, a := range autos {
			if a.Source == "schedule" {
				ov.Schedules = append(ov.Schedules, Schedule{ProjectID: p.ID, Project: p.Name, ID: a.ID, Name: a.Name, Enabled: a.Enabled, LastRunAt: a.LastRunAt, NextRunAt: a.NextRunAt})
			}
		}
	}
	slices.SortStableFunc(ov.Changes, func(a, b Change) int { return b.At.Compare(a.At) })
	if len(ov.Changes) > 100 {
		ov.Changes = ov.Changes[:100]
	}
	return ov, nil
}

// open are the board's statuses of work not finished.
var open = []string{"found", "queued", "handed", "doing", "paused", "review"}

// Report is the overview as text, for an agent's report.
func (ov Overview) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Since %s: %d runs, %.2f USD.\n\nWork boards (open tasks by status; added / done / failed since; cost):\n", ov.Since.Format("2006-01-02 15:04"), ov.Runs, ov.CostUSD)
	for _, p := range ov.Projects {
		var st []string
		for _, s := range open {
			if n := p.ByStatus[s]; n > 0 {
				st = append(st, fmt.Sprintf("%s %d", s, n))
			}
		}
		if len(st) == 0 {
			st = []string{"none open"}
		}
		fmt.Fprintf(&b, "- %s: %s; +%d / done %d / failed %d; %.2f USD\n", p.Project, strings.Join(st, ", "), p.Added, p.Done, p.Failed, p.CostUSD)
	}
	b.WriteString("\nAgents (runs, errors, USD):\n")
	for _, a := range ov.Agents {
		fmt.Fprintf(&b, "- %s / %s: %d runs, %d errors, %.2f USD\n", a.Project, a.Agent, a.Runs, a.Errors, a.CostUSD)
	}
	b.WriteString("\nSetup changes:\n")
	if len(ov.Changes) == 0 {
		b.WriteString("- none\n")
	}
	for i, c := range ov.Changes {
		if i == 30 {
			fmt.Fprintf(&b, "- … %d more\n", len(ov.Changes)-30)
			break
		}
		fmt.Fprintf(&b, "- %s %s: %s %s %s\n", c.At.Format("01-02 15:04"), c.Project, c.Who, c.Action, c.Target)
	}
	b.WriteString("\nSchedules:\n")
	for _, s := range ov.Schedules {
		state, last := "off", "never"
		if s.Enabled {
			state = "on"
		}
		if s.LastRunAt != nil {
			last = s.LastRunAt.Format("01-02 15:04")
		}
		fmt.Fprintf(&b, "- %s / %s: %s, last run %s\n", s.Project, s.Name, state, last)
	}
	return b.String()
}
