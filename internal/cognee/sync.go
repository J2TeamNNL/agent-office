package cognee

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tung1998/agent-office/internal/storage"
)

// Sync keeps each project's dataset what its agents' notes are. The notes
// change in many places (tools, dashboard, compaction, restore, import), so
// it compares rather than hooks: each round, what was sent is checked
// against the notes now.
type Sync struct {
	store storage.Store
	// resolve finds the server each round (nil = client, fixed)
	resolve func(ctx context.Context) *Client
	mu      sync.Mutex
	client  *Client // nil = off
}

func NewSync(store storage.Store, c *Client) *Sync { return &Sync{store: store, client: c} }

func (s *Sync) current() *Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

func stateKey(projectID string) string { return "cognee_sync/" + projectID }

// doc is one note as Cognee keeps it: who wrote it and its topic, then the
// note, so a hit reads on its own.
type doc struct {
	id, text string
	nodeSet  []string
}

// Run syncs every project now, then every interval, until ctx ends.
func (s *Sync) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		s.All(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// All syncs every project; a failed one is tried again next round.
func (s *Sync) All(ctx context.Context) {
	if s.resolve != nil {
		c := s.resolve(ctx)
		s.mu.Lock()
		s.client = c
		s.mu.Unlock()
		if c != nil && c.port > 0 && !Healthy(ctx, c.URL) {
			if err := Up(ctx, c.port); err != nil {
				slog.Warn("cognee: start", "err", err)
				return
			}
		}
	}
	if s.current() == nil {
		return
	}
	repos, err := s.store.Repos().List(ctx)
	if err != nil {
		return
	}
	for _, r := range repos {
		if err := s.Project(ctx, r.ID); err != nil {
			slog.Warn("cognee: sync", "project", r.ID, "err", err)
		}
	}
}

// Project brings the project's dataset up to its notes: only new notes are
// added; a note edited or gone rebuilds the dataset (Cognee cannot edit one
// item's graph in place).
func (s *Sync) Project(ctx context.Context, projectID string) error {
	client := s.current()
	if client == nil {
		return nil
	}
	docs, err := s.docs(ctx, projectID)
	if err != nil {
		return err
	}
	now := map[string]string{}
	for _, d := range docs {
		now[d.id] = hash(d.text)
	}
	sent := map[string]string{}
	_, _ = s.store.Settings().Get(ctx, stateKey(projectID), &sent)
	if maps.Equal(now, sent) {
		return nil
	}
	rebuild := false
	for id, h := range sent {
		if now[id] != h {
			rebuild = true
			break
		}
	}
	ds := Dataset(projectID)
	if rebuild {
		if err := client.Forget(ctx, ds); err != nil {
			return err
		}
		sent = map[string]string{}
		// what was there is gone: written down before adding, so a failure
		// below starts from empty instead of believing it is still there
		if err := s.store.Settings().Set(ctx, stateKey(projectID), sent); err != nil {
			return err
		}
	}
	groups := map[string][]doc{} // one call per node set
	for _, d := range docs {
		if _, ok := sent[d.id]; !ok {
			k := strings.Join(d.nodeSet, "\x00")
			groups[k] = append(groups[k], d)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(groups)) {
		g := groups[k]
		texts := make([]string, len(g))
		for i, d := range g {
			texts[i] = d.text
		}
		if err := client.Remember(ctx, ds, texts, g[0].nodeSet); err != nil {
			return err
		}
		for _, d := range g {
			sent[d.id] = now[d.id]
		}
		if err := s.store.Settings().Set(ctx, stateKey(projectID), sent); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sync) docs(ctx context.Context, projectID string) ([]doc, error) {
	agents, err := s.store.Agents().List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var out []doc
	for _, a := range agents {
		list, err := s.store.Memories().List(ctx, projectID, a.ID)
		if err != nil {
			return nil, err
		}
		name := cmp.Or(a.Name, a.Key, a.ID)
		for _, m := range list {
			topic := cmp.Or(m.Topic, "core")
			out = append(out, doc{id: m.ID, text: "[" + name + " · " + topic + "] " + m.Text,
				nodeSet: []string{"agent:" + name, "topic:" + topic}})
		}
	}
	return out, nil
}

func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// Related is the project's notes nearest query by meaning, of every agent
// ("[agent · topic] note" each), leaving out those skip already shows.
func (s *Sync) Related(ctx context.Context, projectID, query, skip string, n int) ([]string, error) {
	client := s.current()
	if client == nil {
		return nil, nil // off
	}
	hits, err := client.Recall(ctx, Dataset(projectID), query, n)
	if errors.Is(err, ErrNoDataset) {
		return nil, nil // nothing kept yet
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, h := range hits {
		t := strings.TrimSpace(h.Text)
		if t == "" || slices.Contains(out, t) {
			continue
		}
		if _, note, ok := strings.Cut(t, "] "); ok && strings.Contains(skip, note) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}
