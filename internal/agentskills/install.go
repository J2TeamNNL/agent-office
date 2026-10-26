package agentskills

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tung1998/agent-office/internal/automation"
	"github.com/tung1998/agent-office/internal/storage"
)

// findLimit caps the library skills find_skills lists.
const findLimit = 15

// Installer puts skills of the office library into a project for an agent
// (ADR-187): the agent finds one, proposes it, and once a person approves
// the card it is installed in the project and joins the agent's skills.
type Installer struct {
	Store storage.Store
	Auto  *automation.Service
	Home  string
}

// Find lists the library skills matching every word of query (all of them
// when empty), saying which ones the project already has.
func (i Installer) Find(ctx context.Context, projectID, query string) (string, error) {
	p, err := i.Store.Repos().Get(ctx, projectID)
	if err != nil {
		return "", err
	}
	list, err := i.Auto.Library.List("skill")
	if err != nil {
		return "", err
	}
	have := map[string]bool{}
	for _, s := range automation.ProjectSkills(i.Home, p.Path) {
		have[s.Name] = true
	}
	words := strings.Fields(strings.ToLower(query))
	var lines []string
	more := 0
	for _, it := range list {
		hay := strings.ToLower(it.Name + " " + it.Description + " " + it.Category + " " + strings.Join(it.Tags, " "))
		if slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(hay, w) }) {
			continue
		}
		if len(lines) >= findLimit {
			more++
			continue
		}
		l := "- " + it.Name + ": " + short(it.Description, 160)
		if have[it.Name] {
			l += " (in the project)"
		}
		lines = append(lines, l)
	}
	if len(lines) == 0 {
		return "No skill of the office library matches. Try other words.", nil
	}
	out := strings.Join(lines, "\n")
	if more > 0 {
		out += fmt.Sprintf("\n- …%d more: narrow the words", more)
	}
	return out + "\nAdd one with my_skills: one the project lacks becomes a card the person approves.", nil
}

// Has reports whether the office library has a skill of that name.
func (i Installer) Has(name string) bool {
	_, err := i.Auto.Library.Get("skill", name)
	return err == nil
}

// Check says whether a library skill may be proposed for the project.
func (i Installer) Check(ctx context.Context, projectID, name string) error {
	p, err := i.Store.Repos().Get(ctx, projectID)
	if err != nil {
		return err
	}
	if p.Path == "" {
		return errors.New("project không có thư mục để cài skill")
	}
	if !i.Has(name) {
		return fmt.Errorf("%w: %s", automation.ErrUnknownSkill, name)
	}
	return nil
}

// Install puts the library skill into the project (one already there stays
// as it is) and on the agent's skills.
func (i Installer) Install(ctx context.Context, projectID, agentID, name, why string) (string, error) {
	p, err := i.Store.Repos().Get(ctx, projectID)
	if err != nil {
		return "", err
	}
	res, err := i.Auto.Install(ctx, automation.InstallRequest{Kind: "skill", Library: name,
		Target: automation.Target{Scope: "project", ProjectPath: p.Path}})
	detail := res.Path
	switch {
	case errors.Is(err, automation.ErrExists):
		detail = "đã có trong project"
	case errors.Is(err, automation.ErrNeedsConsent), errors.Is(err, automation.ErrUnsafeSkill):
		return "", fmt.Errorf("%w: xem và cài tay ở trang Tự động hóa", err)
	case err != nil:
		return "", err
	}
	if err := Add(ctx, i.Store, i.Home, p.Path, agentID, name, why, "agent"); err != nil {
		return "", err
	}
	return detail, nil
}
