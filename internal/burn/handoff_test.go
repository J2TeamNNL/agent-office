package burn_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tung1998/agent-office/internal/burn"
	"github.com/tung1998/agent-office/internal/chat"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/storage/sqlite"
)

// ADR-210: a task of the board handed to an agent with delegate is in
// progress outside the Burn, then done with the agent's answer; a task of
// another project's board, or one the Burn is doing, is left as it is.
func TestBoardHandoff(t *testing.T) {
	ctx := context.Background()
	st, _ := sqlite.Open(filepath.Join(t.TempDir(), "o.db"))
	defer st.Close()
	st.Migrate(ctx)
	p, _ := st.Repos().Create(ctx, storage.Repo{Name: "p", Path: t.TempDir()})
	other, _ := st.Repos().Create(ctx, storage.Repo{Name: "o", Path: t.TempDir()})
	b, err := st.Burn().SaveSession(ctx, storage.BurnSession{ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	add := func(status string) storage.BurnItem {
		it, err := st.Burn().AddItem(ctx, storage.BurnItem{SessionID: b.ID, Title: "t-" + status, Kind: burn.KindQuest, Status: status})
		if err != nil {
			t.Fatal(err)
		}
		return it
	}
	svc := burn.New(st, nil, nil)
	task, busy := add("found"), add("doing")
	svc.OnBoard(ctx, chat.BoardEvent{ProjectID: other.ID, Task: task.ID, Agent: "Dev"})
	if it, _ := st.Burn().Item(ctx, task.ID); it.Status != "found" {
		t.Fatalf("another project's hand-off moved it: %s", it.Status)
	}
	svc.OnBoard(ctx, chat.BoardEvent{ProjectID: p.ID, Task: task.ID, Agent: "Dev"})
	if it, _ := st.Burn().Item(ctx, task.ID); it.Status != burn.StatusHanded {
		t.Fatalf("handed: %s", it.Status)
	}
	svc.OnBoard(ctx, chat.BoardEvent{ProjectID: p.ID, Task: task.ID, Agent: "Dev", Done: true, Result: "đã sửa"})
	if it, _ := st.Burn().Item(ctx, task.ID); it.Status != "done" || it.Summary != "Dev: đã sửa" {
		t.Fatalf("done: %s %q", it.Status, it.Summary)
	}
	svc.OnBoard(ctx, chat.BoardEvent{ProjectID: p.ID, Task: busy.ID, Agent: "Dev"})
	if it, _ := st.Burn().Item(ctx, busy.ID); it.Status != "doing" {
		t.Fatalf("took the Burn's piece: %s", it.Status)
	}
	failed := add("queued")
	svc.OnBoard(ctx, chat.BoardEvent{ProjectID: p.ID, Task: failed.ID, Agent: "Dev"})
	svc.OnBoard(ctx, chat.BoardEvent{ProjectID: p.ID, Task: failed.ID, Agent: "Dev", Done: true, Failed: true})
	if it, _ := st.Burn().Item(ctx, failed.ID); it.Status != "failed" {
		t.Fatalf("failed: %s", it.Status)
	}
}
