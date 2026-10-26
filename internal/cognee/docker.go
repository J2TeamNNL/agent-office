package cognee

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tung1998/agent-office/internal/storage"
)

// Office runs Cognee itself in Docker when turned on (`office cognee up`):
// one container on loopback, its data in named volumes that outlive it.
const (
	Container = "agent-office-cognee"
	Image     = "cognee/cognee:main"
	// DefaultPort: 8000 (Cognee's own) is often taken on a dev machine.
	DefaultPort = 18765
	settingKey  = "cognee"
	// setup: bumped when the container's options below change, so a
	// container set up otherwise is made anew
	setup = "3"
)

// Setting is whether office runs Cognee in Docker, and on which port.
type Setting struct {
	Docker bool `json:"docker"`
	Port   int  `json:"port,omitempty"`
}

func Load(ctx context.Context, st storage.Store) Setting {
	var s Setting
	_, _ = st.Settings().Get(ctx, settingKey, &s)
	if s.Port == 0 {
		s.Port = DefaultPort
	}
	return s
}

func Save(ctx context.Context, st storage.Store, s Setting) error {
	return st.Settings().Set(ctx, settingKey, s)
}

func localURL(port int) string { return "http://127.0.0.1:" + strconv.Itoa(port) }

// Auto is the sync office runs: its server is read again each round, so
// `office cognee up|down` take effect without restarting office. COGNEE_URL
// (a server run elsewhere) first, else its own container when turned on,
// started again when it does not answer; none = off, nothing sent.
func Auto(st storage.Store) *Sync {
	return &Sync{store: st, resolve: func(ctx context.Context) *Client {
		if c := FromEnv(); c != nil {
			return c
		}
		s := Load(ctx, st)
		if !s.Docker {
			return nil
		}
		return &Client{URL: localURL(s.Port), HTTP: &http.Client{Timeout: 2 * time.Minute}, port: s.Port}
	}}
}

// Up makes the container run on port, created anew when missing or set up
// otherwise (the volumes, so the data, are kept), then waits until it answers.
func Up(ctx context.Context, port int) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("cognee: không thấy lệnh docker trên máy")
	}
	want := strconv.Itoa(port)
	out, err := docker(ctx, "inspect", "-f", `{{.State.Running}} {{index .Config.Labels "agent-office.setup"}} {{(index (index .HostConfig.PortBindings "8000/tcp") 0).HostPort}}`, Container)
	switch {
	case err == nil && out == "true "+setup+" "+want:
	case err == nil && strings.HasSuffix(out, " "+setup+" "+want):
		if _, err := docker(ctx, "start", Container); err != nil {
			return err
		}
	default:
		_, _ = docker(ctx, "rm", "-f", Container) // gone already, or set up otherwise
		args := []string{"run", "-d", "--name", Container, "--restart", "unless-stopped",
			"--label", "agent-office.setup=" + setup, "-p", "127.0.0.1:" + want + ":8000",
			// graph, vectors, its database, and the embedding model downloaded
			// once (the image's own cache is not writable by its user)
			"-v", Container + ":/cognee-storage", "-e", "HF_HOME=/cognee-storage/huggingface",
			// the notes are mostly Vietnamese: a small multilingual model, local, no key
			"-e", "EMBEDDING_PROVIDER=fastembed", "-e", "EMBEDDING_MODEL=sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2",
			"-e", "EMBEDDING_DIMENSIONS=384",
			// no LLM key: the graph comes from the local GLiNER model; one
			// inference at a time keeps it within a small Docker VM
			"-e", "GLINER_INFERENCE_THREADS=1",
			// one person's machine on loopback: no login, no tenants
			"-e", "REQUIRE_AUTHENTICATION=false", "-e", "ENABLE_BACKEND_ACCESS_CONTROL=false"}
		if os.Getenv("LLM_API_KEY") != "" { // the graph by an LLM instead: far less RAM
			args = append(args, "-e", "LLM_API_KEY") // its value from office's environment, not on the command line
		}
		if _, err := docker(ctx, append(args, Image)...); err != nil {
			return err
		}
	}
	return wait(ctx, localURL(port), 5*time.Minute)
}

// Down stops and removes the container; its volumes (the data) stay.
func Down(ctx context.Context) error {
	if _, err := docker(ctx, "rm", "-f", Container); err != nil && !strings.Contains(err.Error(), "No such container") {
		return err
	}
	return nil
}

// Status is the container's state ("" = none) and whether Cognee answers.
func Status(ctx context.Context, port int) (state string, ok bool) {
	state, _ = docker(ctx, "inspect", "-f", "{{.State.Status}}", Container)
	return state, Healthy(ctx, localURL(port))
}

// Healthy: Cognee answers on url (its /health is 200 once the API is up).
func Healthy(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/health", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

func wait(ctx context.Context, url string, limit time.Duration) error {
	end := time.Now().Add(limit)
	for !Healthy(ctx, url) {
		if time.Now().After(end) {
			return fmt.Errorf("cognee: %s chưa trả lời sau %s (xem docker logs %s)", url, limit, Container)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return nil
}

func docker(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("docker %s: %s", args[0], s)
	}
	return s, nil
}
