package actions

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tung1998/agent-office/internal/memory"
	"github.com/tung1998/agent-office/internal/perm"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/storage/sqlite"
	"github.com/tung1998/agent-office/internal/team"
)

// An agent's "remember" is kept on its own (unless the project turned that
// off), within guards: too short or already noted is refused; no reason, or
// more than a turn's share, waits for a person.
func TestRemember(t *testing.T) {
	ctx := context.Background()
	st, _ := sqlite.Open(filepath.Join(t.TempDir(), "o.db"))
	defer st.Close()
	st.Migrate(ctx)
	org := team.NewService(st, nil)
	proj, _ := st.Repos().Create(ctx, storage.Repo{Name: "p", Path: t.TempDir()})
	org.AddFromLibrary(ctx, proj.ID, team.StarterAgent)
	agents, _ := st.Agents().List(ctx, proj.ID)
	mem := memory.New(st, nil)
	svc := New(st, nil)
	svc.SetMemory(mem)
	mem.SetAuto(ctx, proj.ID, false)
	sc := Scope{ProjectID: proj.ID, RunRef: "r1", Agent: agents[0].Name, Level: perm.Propose}
	a, err := svc.Propose(ctx, sc, "remember", "project dùng pnpm, không dùng npm", "thấy lockfile")
	if err != nil || a.Status != "pending" || a.TargetID != agents[0].ID {
		t.Fatalf("propose = %+v %v", a, err)
	}
	if _, err := svc.Decide(ctx, a.ID, true, "human:a"); err != nil {
		t.Fatal(err)
	}
	list, _ := st.Memories().List(ctx, proj.ID, agents[0].ID)
	if len(list) != 1 || list[0].Text != "project dùng pnpm, không dùng npm" || list[0].Source != "agent" {
		t.Fatalf("notes = %+v", list)
	}
	mem.SetAuto(ctx, proj.ID, true)
	sc.RunRef = "r2"
	if a, _ = svc.Propose(ctx, sc, "remember", "chạy pnpm test trước khi báo xong", "bị nhắc"); a.Status != "done" {
		t.Fatalf("auto = %+v", a)
	}
	if _, err := svc.Propose(ctx, sc, "remember", "Project dùng pnpm — không dùng npm.", "x"); err == nil {
		t.Fatal("the same note twice")
	}
	if _, err := svc.Propose(ctx, sc, "remember", "dùng pnpm", "x"); err == nil {
		t.Fatal("a note too short to help")
	}
	if a, _ = svc.Propose(ctx, sc, "remember", "migration nằm ở internal/storage/sqlite/migrations", ""); a.Status != "pending" {
		t.Fatalf("no reason = %+v", a)
	}
	if a, _ = svc.Propose(ctx, sc, "remember", "log của office ở .office/logs/office.log", "tìm lâu"); a.Status != "done" {
		t.Fatalf("second of the turn = %+v", a)
	}
	if a, _ = svc.Propose(ctx, sc, "remember", "build dashboard bằng pnpm --dir dashboard build", "tìm lâu"); a.Status != "pending" {
		t.Fatalf("over the turn's share = %+v", a)
	}
	if _, err := svc.Propose(ctx, Scope{ProjectID: proj.ID, Agent: "không có", Level: perm.Propose}, "remember", "một ghi chú đủ dài để có ích", ""); err == nil {
		t.Fatal("a note of an agent that is not in the project")
	}
}

// A chat in direct mode approves what the agent proposes at once (the agent
// gets the result in the same turn and goes on), unless the chat says no.
func TestDirectApprover(t *testing.T) {
	ctx := context.Background()
	st, _ := sqlite.Open(filepath.Join(t.TempDir(), "o.db"))
	defer st.Close()
	st.Migrate(ctx)
	proj, _ := st.Repos().Create(ctx, storage.Repo{Name: "p", Path: t.TempDir()})
	job, _ := st.Jobs().Create(ctx, storage.Job{ProjectID: proj.ID, Kind: "chat_turn", Origin: "automation", Trigger: "discord", Status: "running"})
	svc := New(st, nil)
	var asked []string
	svc.SetAutoApprover(func(_ context.Context, a storage.Action) (string, bool) {
		asked = append(asked, a.Kind+" "+a.Target)
		return "discord:an", a.Target != "echo không"
	})
	sc := Scope{ProjectID: proj.ID, RunRef: "r1", JobID: job.ID, Agent: "a", Level: perm.Propose}
	a, err := svc.Propose(ctx, sc, "run_command", "echo có", "thử")
	if err != nil || a.Status != "done" || a.DecidedBy != "discord:an" || !strings.Contains(a.Detail, "có") {
		t.Fatalf("direct = %+v %v", a, err)
	}
	sc.RunRef = "r2"
	if a, _ = svc.Propose(ctx, sc, "run_command", "echo không", "thử"); a.Status != "pending" {
		t.Fatalf("refused by the chat = %+v", a)
	}
	sc.RunRef, sc.JobID = "r3", ""
	if a, _ = svc.Propose(ctx, sc, "run_command", "echo web", "thử"); a.Status != "pending" || len(asked) != 2 {
		t.Fatalf("no job, no chat to ask: %+v %v", a, asked)
	}
}
