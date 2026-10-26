package api_test

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ADR-194: an agent whose job changes (here its role and permission level)
// is told so in a chat of its own, to bring its notes and schedules up to
// date; an edit that changes nothing of its job tells it nothing.
func TestRoleChangeBriefsTheAgent(t *testing.T) {
	e := setup(t)
	admin := e.client(t)
	login(t, e, admin, "admin@x.io", "admin-password")
	_, body := do(t, admin, "POST", e.srv.URL+"/api/projects", map[string]any{"path": t.TempDir(), "name": "shop"}, nil)
	project := body["project"].(map[string]any)
	a := project["agents"].([]any)[0].(map[string]any)
	id := a["id"].(string)
	put := func(change map[string]any) {
		t.Helper()
		in := map[string]any{}
		for _, k := range []string{"key", "name", "role", "description", "provider_id", "model_tier", "llm_model", "instructions", "permissions"} {
			in[k] = a[k]
		}
		for k, v := range change {
			in[k] = v
		}
		if resp, b := do(t, admin, "PATCH", e.srv.URL+"/api/agents/"+id, in, nil); resp.StatusCode != 200 {
			t.Fatalf("PATCH = %d %v", resp.StatusCode, b)
		}
	}
	briefs := func() []string {
		var out []string
		convs, _ := e.st.Chat().ListConversationsFrom(context.Background(), project["id"].(string), "all", 50)
		for _, c := range convs {
			msgs, _ := e.st.Chat().ListMessages(context.Background(), c.ID)
			for _, m := range msgs {
				if strings.HasPrefix(m.Content, "[Office] Your job changed") {
					out = append(out, m.Content)
				}
			}
		}
		return out
	}
	put(map[string]any{"description": "chỉ đổi mô tả"})
	time.Sleep(200 * time.Millisecond)
	if n := len(briefs()); n != 0 {
		t.Fatalf("a description edit briefed the agent %d times", n)
	}
	put(map[string]any{"role": "Trưởng phòng kinh doanh", "permissions": map[string]any{"level": "read", "read_only": true}})
	var got []string
	for i := 0; i < 50 && len(got) == 0; i++ {
		time.Sleep(40 * time.Millisecond)
		got = briefs()
	}
	if len(got) != 1 || !strings.Contains(got[0], "Trưởng phòng kinh doanh") || !strings.Contains(got[0], "permission level") || !strings.Contains(got[0], "only_if") {
		t.Fatalf("brief: %q", got)
	}
}
