package api

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tung1998/agent-office/internal/agentskills"
	"github.com/tung1998/agent-office/internal/audit"
	"github.com/tung1998/agent-office/internal/automation"
	"github.com/tung1998/agent-office/internal/storage"
)

// agentSkillDTO is one skill an agent keeps at hand (ADR-185).
type agentSkillDTO struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Why         string     `json:"why"`
	Source      string     `json:"source"`
	Uses        int        `json:"uses"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time  `json:"created_at"`
	Missing     bool       `json:"missing"` // no longer in the project: left out of the prompt
}

func (s *server) listAgentSkills(w http.ResponseWriter, r *http.Request) {
	pid, aid, ok := s.projectAgent(w, r)
	if !ok {
		return
	}
	x, err := s.cfg.Store.Repos().Get(r.Context(), pid)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	list, err := s.cfg.Store.AgentSkills().List(r.Context(), aid)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	home, _ := os.UserHomeDir()
	desc := map[string]string{}
	for _, sk := range automation.ProjectSkills(home, x.Path) {
		desc[sk.Name] = sk.Description
	}
	items := make([]agentSkillDTO, 0, len(list))
	for _, sk := range list {
		d, found := desc[sk.Name]
		it := agentSkillDTO{Name: sk.Name, Description: d, Why: sk.Why, Source: sk.Source, Uses: sk.Uses, CreatedAt: sk.CreatedAt, Missing: !found}
		if !sk.LastUsedAt.IsZero() {
			t := sk.LastUsedAt
			it.LastUsedAt = &t
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "limit": agentskills.Limit})
}

func (s *server) addAgentSkill(w http.ResponseWriter, r *http.Request) {
	pid, aid, ok := s.projectAgent(w, r)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
		Why  string `json:"why"`
	}
	if !decode(w, r, &in) {
		return
	}
	x, err := s.cfg.Store.Repos().Get(r.Context(), pid)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	home, _ := os.UserHomeDir()
	if err := agentskills.Add(r.Context(), s.cfg.Store, home, x.Path, aid, in.Name, in.Why, "person"); err != nil {
		if errors.Is(err, agentskills.ErrFull) || errors.Is(err, automation.ErrUnknownSkill) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.internal(w, r, err)
		return
	}
	s.audit(r, audit.Change{Action: "agent.skill_add", Resource: "agent", ResourceID: aid, ProjectID: pid, Detail: map[string]any{"skill": strings.TrimSpace(in.Name)}})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *server) removeAgentSkill(w http.ResponseWriter, r *http.Request) {
	pid, aid, ok := s.projectAgent(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if err := agentskills.Remove(r.Context(), s.cfg.Store, aid, name); err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	s.audit(r, audit.Change{Action: "agent.skill_remove", Resource: "agent", ResourceID: aid, ProjectID: pid, Detail: map[string]any{"skill": name}})
	w.WriteHeader(http.StatusNoContent)
}

// createAgentSkills gives a new agent the skills it came with: one the
// project lacks is installed from the office library first, as the setup
// does; one that cannot be is left out.
func (s *server) createAgentSkills(r *http.Request, a storage.Agent, names []string) {
	if len(names) == 0 {
		return
	}
	ctx := r.Context()
	x, err := s.cfg.Store.Repos().Get(ctx, a.ProjectID)
	if err != nil {
		return
	}
	home, _ := os.UserHomeDir()
	have := map[string]bool{}
	for _, sk := range automation.ProjectSkills(home, x.Path) {
		have[sk.Name] = true
	}
	for i, name := range cleanList(names) {
		if i >= agentskills.Limit {
			break
		}
		if !have[name] {
			if it := s.setupSkill(ctx, x, name); it.Status != "ok" {
				slog.Warn("agent skill: install", "agent", a.Key, "skill", name, "detail", it.Detail)
				continue
			}
		}
		if err := agentskills.Add(ctx, s.cfg.Store, home, x.Path, a.ID, name, "", "library"); err != nil {
			slog.Warn("agent skill", "agent", a.Key, "skill", name, "err", err)
		}
	}
}
