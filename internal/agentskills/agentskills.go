// Package agentskills keeps the skills each agent has at hand (ADR-185): the
// agent's prompt lists their names, why and how often they were used, and
// the agent reads a skill's instructions with use_skill only when a task
// calls for it. The list grows from the setup, the agent (my_skills) and the
// person calling a skill with "/name" in the agent's chat; it is capped, so
// skills that earn no use make room for others.
package agentskills

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tung1998/agent-office/internal/automation"
	"github.com/tung1998/agent-office/internal/prompts"
	"github.com/tung1998/agent-office/internal/storage"
)

// Limit caps the skills an agent keeps.
const Limit = 12

// staleAfter: a skill unused this long is flagged in the prompt.
const staleAfter = 30 * 24 * time.Hour

// ErrFull: the list is at Limit; one must go before another comes.
var ErrFull = fmt.Errorf("danh sách skill đã đủ %d: bỏ một skill ít dùng trước", Limit)

// Block is the agent's skills as its system prompt gets them ("" when the
// project has no skill at all).
func Block(ctx context.Context, store storage.Store, home, projectPath, agentID string) string {
	all := automation.ProjectSkills(home, projectPath)
	if len(all) == 0 {
		return ""
	}
	list, err := store.AgentSkills().List(ctx, agentID)
	if err != nil {
		return ""
	}
	desc := map[string]string{}
	for _, s := range all {
		desc[s.Name] = s.Description
	}
	var items []string
	for _, s := range list {
		d, ok := desc[s.Name]
		if !ok {
			continue // removed from the project: not offered
		}
		items = append(items, line(s, d, time.Now()))
	}
	return "\n\n" + prompts.Render("skills/own", map[string]any{"Items": items, "Available": len(all), "Limit": Limit})
}

// line is one skill of the prompt: name, what it does, why the agent keeps
// it, how it was used.
func line(s storage.AgentSkill, desc string, now time.Time) string {
	b := s.Name
	if d := short(desc, 140); d != "" {
		b += ": " + d
	}
	if s.Why != "" {
		b += " Why: " + short(s.Why, 140)
	}
	switch {
	case s.Uses == 0 && now.Sub(s.CreatedAt) > staleAfter:
		b += " (never used in 30 days)"
	case s.Uses == 0:
		b += " (not used yet)"
	case now.Sub(s.LastUsedAt) > staleAfter:
		b += fmt.Sprintf(" (used %d×, none in 30 days)", s.Uses)
	default:
		b += fmt.Sprintf(" (used %d×, last %s)", s.Uses, s.LastUsedAt.Format("2006-01-02"))
	}
	return b
}

func short(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// Add puts a project skill on the agent's list (one already there gets the
// new why). A full list refuses: the agent weighs which one to drop.
func Add(ctx context.Context, store storage.Store, home, projectPath, agentID, name, why, source string) error {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if _, _, _, err := automation.LoadSkill(home, projectPath, name); err != nil {
		return err
	}
	list, err := store.AgentSkills().List(ctx, agentID)
	if err != nil {
		return err
	}
	have := false
	for _, s := range list {
		have = have || s.Name == name
	}
	if !have && len(list) >= Limit {
		return ErrFull
	}
	return store.AgentSkills().Add(ctx, storage.AgentSkill{AgentID: agentID, Name: name, Why: short(why, 200), Source: source})
}

// Remove takes a skill off the agent's list.
func Remove(ctx context.Context, store storage.Store, agentID, name string) error {
	return store.AgentSkills().Remove(ctx, agentID, strings.TrimPrefix(strings.TrimSpace(name), "/"))
}

// Use reads a skill's instructions for the agent and counts the use when
// the skill is on its list.
func Use(ctx context.Context, store storage.Store, home, projectPath, agentID, name, task string) (string, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	skill, body, others, err := automation.LoadSkill(home, projectPath, name)
	if err != nil {
		return "", err
	}
	onList, err := store.AgentSkills().Used(ctx, agentID, name)
	if err != nil {
		return "", err
	}
	return prompts.Render("skills/use", map[string]any{"Name": name, "Dir": skill.Path, "Body": body, "Others": others,
		"Task": cmp.Or(strings.TrimSpace(task), "(not given)"), "OnList": onList}), nil
}

// Called counts a skill the person called with "/name" in the agent's chat:
// one on the list gets a use, one not on it joins while there is room.
func Called(ctx context.Context, store storage.Store, home, projectPath, agentID, name string) {
	if ok, err := store.AgentSkills().Used(ctx, agentID, name); ok || err != nil {
		return
	}
	if Add(ctx, store, home, projectPath, agentID, name, "", "call") == nil { // a full list stays as the agent keeps it
		_, _ = store.AgentSkills().Used(ctx, agentID, name)
	}
}
