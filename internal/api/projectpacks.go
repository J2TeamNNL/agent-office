package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/tung1998/agent-office/internal/repos"
	"github.com/tung1998/agent-office/templates/packs"
)

// The library's project packs (ADR-199), for the dashboard's Library and an
// agent's list resource project_pack: each pack with a link to its source in
// office's own repo, when that repo's remote is a GitHub or Bitbucket one.

type projectPackDTO struct {
	packs.Entry
	Bundle string `json:"bundle"`
	Source string `json:"source,omitempty"`
}

func (s *server) projectPacks(w http.ResponseWriter, r *http.Request) {
	list, err := s.packList(r.Context())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	bundles, _ := packs.Bundles()
	writeJSON(w, http.StatusOK, map[string]any{"bundles": bundles, "packs": list})
}

// packList is every pack of the library, with its source link.
func (s *server) packList(_ context.Context) ([]projectPackDTO, error) {
	bundles, err := packs.Bundles()
	if err != nil {
		return nil, err
	}
	base := ""
	if root := s.cfg.System.ProjectRoot; root != "" {
		if info, err := repos.Detect(root); err == nil {
			base = sourceBase(info.GitRemote)
		}
	}
	out := []projectPackDTO{}
	for _, b := range bundles {
		for _, e := range b.Packs {
			d := projectPackDTO{Entry: e, Bundle: b.Name}
			if base != "" {
				d.Source = base + e.Path
			}
			out = append(out, d)
		}
	}
	return out, nil
}

// sourceBase is where a file of a repo is browsed on GitHub or Bitbucket
// (its default branch), "" for any other remote.
func sourceBase(remote string) string {
	remote = strings.TrimSuffix(strings.TrimSpace(remote), ".git")
	host, path := "", ""
	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		host, path = u.Hostname(), strings.TrimPrefix(u.Path, "/")
	} else if at := strings.LastIndex(remote, "@"); at >= 0 { // git@host:owner/repo
		host, path, _ = strings.Cut(remote[at+1:], ":")
	}
	if strings.Count(path, "/") != 1 {
		return ""
	}
	switch host {
	case "github.com":
		return "https://github.com/" + path + "/blob/HEAD/"
	case "bitbucket.org":
		return "https://bitbucket.org/" + path + "/src/HEAD/"
	}
	return ""
}
