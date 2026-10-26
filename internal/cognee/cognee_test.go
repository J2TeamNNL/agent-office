package cognee_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tung1998/agent-office/internal/cognee"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/storage/sqlite"
)

// fake is a Cognee server: datasets of texts; recall returns those sharing
// a word with the query.
type fake struct {
	mu       sync.Mutex
	data     map[string][]string
	nodeSets [][]string
	calls    []string
	key      string
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.URL.Path)
	f.key = r.Header.Get("X-Api-Key")
	switch r.URL.Path {
	case "/api/v1/remember":
		_ = r.ParseMultipartForm(1 << 20)
		ds := r.FormValue("datasetName")
		if r.FormValue("self_improvement") != "false" {
			http.Error(w, "self_improvement", 400)
			return
		}
		f.data[ds] = append(f.data[ds], r.MultipartForm.Value["raw_data"]...)
		f.nodeSets = append(f.nodeSets, r.MultipartForm.Value["node_set"])
		w.Write([]byte(`{}`))
	case "/api/v1/recall":
		var raw map[string]any
		json.NewDecoder(r.Body).Decode(&raw)
		query, _ := raw["query"].(string)
		if raw["search_type"] != "CHUNKS" {
			http.Error(w, "search_type", 400)
			return
		}
		ds := raw["datasets"].([]any)[0].(string)
		texts, ok := f.data[ds]
		if !ok {
			http.Error(w, "no dataset", 404)
			return
		}
		out := []map[string]any{{"source": "system", "kind": "marker", "search_type": "CHUNKS", "text": "Memory is still warming up"}}
		for _, t := range texts {
			for _, w := range strings.Fields(query) {
				if strings.Contains(t, w) {
					out = append(out, map[string]any{"source": "graph", "kind": "chunks", "search_type": "CHUNKS", "text": t})
					break
				}
			}
		}
		json.NewEncoder(w).Encode(out)
	case "/api/v1/forget":
		var raw map[string]any
		json.NewDecoder(r.Body).Decode(&raw)
		ds := raw["dataset"].(string)
		if _, ok := f.data[ds]; !ok {
			http.Error(w, "no dataset", 404)
			return
		}
		delete(f.data, ds)
		w.Write([]byte(`{}`))
	default:
		http.NotFound(w, r)
	}
}

func TestSync(t *testing.T) {
	ctx := context.Background()
	st, _ := sqlite.Open(filepath.Join(t.TempDir(), "o.db"))
	t.Cleanup(func() { st.Close() })
	st.Migrate(ctx)
	p, _ := st.Repos().Create(ctx, storage.Repo{Name: "p", Path: t.TempDir()})
	a, err := st.Agents().Create(ctx, storage.Agent{ProjectID: p.ID, Key: "dev", Name: "Dev", ModelTier: "fast"})
	if err != nil {
		t.Fatal(err)
	}
	f := &fake{data: map[string][]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv(cognee.EnvURL, srv.URL+"/")
	t.Setenv(cognee.EnvKey, "k1")
	s := cognee.NewSync(st, cognee.FromEnv())
	ds := cognee.Dataset(p.ID)

	// nothing kept yet: recall finds nothing, no error
	if got, err := s.Related(ctx, p.ID, "pnpm", "", 5); err != nil || len(got) != 0 {
		t.Fatalf("empty = %v %v", got, err)
	}
	m1, _ := st.Memories().Create(ctx, storage.Memory{ProjectID: p.ID, AgentID: a.ID, Text: "dùng pnpm để build"})
	st.Memories().Create(ctx, storage.Memory{ProjectID: p.ID, AgentID: a.ID, Text: "refund qua PayPal retry 3 lần", Topic: "payments"})
	if err := s.Project(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.data[ds]) != 2 || !slices.Contains(f.data[ds], "[Dev · core] dùng pnpm để build") || f.key != "k1" {
		t.Fatalf("data = %v key %q", f.data[ds], f.key)
	}
	if !slices.ContainsFunc(f.nodeSets, func(n []string) bool { return slices.Equal(n, []string{"agent:Dev", "topic:payments"}) }) {
		t.Fatalf("node sets = %v", f.nodeSets)
	}

	// unchanged: nothing sent
	n := len(f.calls)
	s.Project(ctx, p.ID)
	if len(f.calls) != n {
		t.Fatalf("resent: %v", f.calls[n:])
	}

	// a new note: only it is added
	st.Memories().Create(ctx, storage.Memory{ProjectID: p.ID, AgentID: a.ID, Text: "deploy bằng make ship"})
	s.Project(ctx, p.ID)
	if len(f.data[ds]) != 3 || slices.Contains(f.calls[n:], "/api/v1/forget") {
		t.Fatalf("add: data %v calls %v", f.data[ds], f.calls[n:])
	}

	// an edited note: the dataset is rebuilt
	m1.Text = "dùng pnpm 9 để build"
	st.Memories().Update(ctx, m1)
	s.Project(ctx, p.ID)
	if len(f.data[ds]) != 3 || !slices.Contains(f.data[ds], "[Dev · core] dùng pnpm 9 để build") {
		t.Fatalf("rebuild: %v", f.data[ds])
	}

	// recall: what keyword search already shows is left out
	got, err := s.Related(ctx, p.ID, "pnpm PayPal", "Notes:\n- dùng pnpm 9 để build\n", 5)
	if err != nil || len(got) != 1 || got[0] != "[Dev · payments] refund qua PayPal retry 3 lần" {
		t.Fatalf("related = %v %v", got, err)
	}

	// server down: an error, which recall drops
	srv.Close()
	if _, err := s.Related(ctx, p.ID, "pnpm", "", 5); err == nil {
		t.Fatal("no error with the server down")
	}
}

// Auto reads the server again each round: off, nothing is sent and recall
// adds nothing; turned on (here COGNEE_URL), the running office syncs.
func TestAuto(t *testing.T) {
	ctx := context.Background()
	st, _ := sqlite.Open(filepath.Join(t.TempDir(), "o.db"))
	t.Cleanup(func() { st.Close() })
	st.Migrate(ctx)
	p, _ := st.Repos().Create(ctx, storage.Repo{Name: "p", Path: t.TempDir()})
	a, _ := st.Agents().Create(ctx, storage.Agent{ProjectID: p.ID, Key: "dev", Name: "Dev", ModelTier: "fast"})
	st.Memories().Create(ctx, storage.Memory{ProjectID: p.ID, AgentID: a.ID, Text: "dùng pnpm"})
	f := &fake{data: map[string][]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	t.Setenv(cognee.EnvURL, " ")
	if cognee.FromEnv() != nil {
		t.Fatal("on without a URL")
	}
	s := cognee.Auto(st)
	s.All(ctx)
	if got, err := s.Related(ctx, p.ID, "pnpm", "", 5); len(f.calls) != 0 || got != nil || err != nil {
		t.Fatalf("off: calls %v, related %v %v", f.calls, got, err)
	}
	t.Setenv(cognee.EnvURL, srv.URL)
	s.All(ctx)
	if got, _ := s.Related(ctx, p.ID, "pnpm", "", 5); len(f.data[cognee.Dataset(p.ID)]) != 1 || len(got) != 1 {
		t.Fatalf("on: data %v, related %v", f.data, got)
	}
}
