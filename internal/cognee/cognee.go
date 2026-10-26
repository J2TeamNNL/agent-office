// Package cognee mirrors the agents' notes into a Cognee server (ADR-195)
// so recall also finds notes by meaning, across a project's agents.
//
// The notes stay in SQLite (ADR-068); Cognee is only an index built from
// them: one dataset per project, rebuilt when the notes changed other than
// by new ones. Off unless COGNEE_URL names a server or office runs its own
// in Docker (`office cognee up`); when it is down, recall answers from the
// notes alone.
package cognee

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

// Env names the server ("" = off) and its API key ("" = a local server
// without authentication).
const (
	EnvURL = "COGNEE_URL"
	EnvKey = "COGNEE_API_KEY"
)

// Client talks to Cognee's REST API (v1: remember, recall, forget).
type Client struct {
	URL, Key string
	HTTP     *http.Client
	port     int // office's own container on this port (0 = a server run elsewhere)
}

// FromEnv is the client COGNEE_URL names, nil when it is unset.
func FromEnv() *Client {
	u := strings.TrimRight(strings.TrimSpace(os.Getenv(EnvURL)), "/")
	if u == "" {
		return nil
	}
	return &Client{URL: u, Key: strings.TrimSpace(os.Getenv(EnvKey)), HTTP: &http.Client{Timeout: 2 * time.Minute}}
}

// ErrNoDataset: the dataset is not there (nothing kept yet).
var ErrNoDataset = errors.New("cognee: no such dataset")

// Dataset is a project's dataset name.
func Dataset(projectID string) string { return "office_" + projectID }

// Remember adds texts to dataset, one data item each, tagged with nodeSet.
// Short notes: no self-improvement loop (it costs LLM calls for little), and
// the graph is built in the background.
func (c *Client) Remember(ctx context.Context, dataset string, texts, nodeSet []string) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("datasetName", dataset)
	for _, t := range texts {
		_ = w.WriteField("raw_data", t)
	}
	for _, n := range nodeSet {
		_ = w.WriteField("node_set", n)
	}
	_ = w.WriteField("self_improvement", "false")
	_ = w.WriteField("run_in_background", "true")
	if err := w.Close(); err != nil {
		return err
	}
	return c.do(ctx, "/api/v1/remember", w.FormDataContentType(), &body, nil)
}

// Hit is one note recall found by meaning.
type Hit struct {
	Text   string   `json:"text"`
	Score  *float64 `json:"score"`
	Source string   `json:"source"`
}

// Recall finds the chunks of dataset nearest query (CHUNKS: the notes as
// written, no LLM call to answer).
func (c *Client) Recall(ctx context.Context, dataset, query string, topK int) ([]Hit, error) {
	in, _ := json.Marshal(map[string]any{"query": query, "datasets": []string{dataset},
		"search_type": "CHUNKS", "scope": "graph", "top_k": topK})
	var all []Hit
	if err := c.do(ctx, "/api/v1/recall", "application/json", bytes.NewReader(in), &all); err != nil {
		return nil, err
	}
	var out []Hit
	for _, h := range all {
		if h.Source == "graph" { // not its notices ("still warming up")
			out = append(out, h)
		}
	}
	return out, nil
}

// Forget deletes dataset (gone already is no error).
func (c *Client) Forget(ctx context.Context, dataset string) error {
	in, _ := json.Marshal(map[string]any{"dataset": dataset})
	if err := c.do(ctx, "/api/v1/forget", "application/json", bytes.NewReader(in), nil); err != nil && !errors.Is(err, ErrNoDataset) {
		return err
	}
	return nil
}

func (c *Client) do(ctx context.Context, path, ctype string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", ctype)
	if c.Key != "" {
		req.Header.Set("X-Api-Key", c.Key)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode == http.StatusNotFound {
		return ErrNoDataset
	}
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("cognee %s: %s: %s", path, res.Status, strings.TrimSpace(string(raw[:min(len(raw), 300)])))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}
