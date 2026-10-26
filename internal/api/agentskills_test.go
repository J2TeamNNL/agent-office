package api_test

import (
	"os"
	"path/filepath"
	"testing"
)

// ADR-185: an agent made with skills keeps those the project has; the list
// is read, added to and removed from through its own routes.
func TestAgentSkillsRoutes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	e := setup(t)
	admin := e.client(t)
	login(t, e, admin, "admin@x.io", "admin-password")
	dir := t.TempDir()
	for _, name := range []string{"page-builder", "design-polish"} {
		d := filepath.Join(dir, ".claude", "skills", name)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: "+name+" steps\n---\nbody\n"), 0o644)
	}
	_, body := do(t, admin, "POST", e.srv.URL+"/api/projects", map[string]any{"path": dir, "name": "site"}, nil)
	pid := body["project"].(map[string]any)["id"].(string)
	resp, body := do(t, admin, "POST", e.srv.URL+"/api/projects/"+pid+"/agents", map[string]any{
		"key": "fe", "name": "FE", "model_tier": "balanced", "permissions": map[string]any{"level": "read"},
		"skills": []string{"page-builder", "nope"}}, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("create = %d %v", resp.StatusCode, body)
	}
	aid := body["agent"].(map[string]any)["id"].(string)
	base := e.srv.URL + "/api/projects/" + pid + "/agents/" + aid + "/skills"
	_, body = do(t, admin, "GET", base, nil, nil)
	items := body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["name"] != "page-builder" || items[0].(map[string]any)["source"] != "library" {
		t.Fatalf("skills = %v", body)
	}
	if resp, _ := do(t, admin, "POST", base, map[string]any{"name": "nope"}, nil); resp.StatusCode != 400 {
		t.Fatalf("unknown skill = %d", resp.StatusCode)
	}
	if resp, _ := do(t, admin, "POST", base, map[string]any{"name": "design-polish", "why": "before a release"}, nil); resp.StatusCode != 200 {
		t.Fatalf("add = %d", resp.StatusCode)
	}
	if resp, _ := do(t, admin, "DELETE", base+"/page-builder", nil, nil); resp.StatusCode != 204 {
		t.Fatalf("remove = %d", resp.StatusCode)
	}
	_, body = do(t, admin, "GET", base, nil, nil)
	if items = body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["why"] != "before a release" {
		t.Fatalf("after = %v", body)
	}
}
