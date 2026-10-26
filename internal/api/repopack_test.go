package api_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tung1998/agent-office/internal/transfer"
)

// ADR-190: a folder carrying office.pack.json becomes that project (its
// agents instead of the starter one); a pack that does not read adds nothing.
func TestAddFolderWithPack(t *testing.T) {
	e := setup(t)
	admin := e.client(t)
	login(t, e, admin, "admin@x.io", "admin-password")
	dir := t.TempDir()
	pack := `{"version":2,"include":["config"],"project":{"name":"Acme","agents":{"agents":[{"key":"ceo","name":"Tổng giám đốc","role":"CEO","model_tier":"strong"}]}}}`
	os.WriteFile(filepath.Join(dir, transfer.RepoPackFile), []byte(pack), 0o644)
	resp, body := do(t, admin, "POST", e.srv.URL+"/api/projects", map[string]any{"path": dir, "name": "acme"}, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("add = %d %v", resp.StatusCode, body)
	}
	agents := body["project"].(map[string]any)["agents"].([]any)
	if len(agents) != 1 || agents[0].(map[string]any)["key"] != "ceo" {
		t.Fatalf("agents: %v", agents)
	}

	bad := t.TempDir()
	os.WriteFile(filepath.Join(bad, transfer.RepoPackFile), []byte("{"), 0o644)
	if resp, _ := do(t, admin, "POST", e.srv.URL+"/api/projects", map[string]any{"path": bad, "name": "bad"}, nil); resp.StatusCode != 400 {
		t.Fatalf("bad pack = %d", resp.StatusCode)
	}
	_, body = do(t, admin, "GET", e.srv.URL+"/api/projects", nil, nil)
	if n := len(body["projects"].([]any)); n != 1 {
		t.Fatalf("projects: %d", n)
	}
}
