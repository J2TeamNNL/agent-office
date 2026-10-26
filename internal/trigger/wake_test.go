package trigger_test

import (
	"context"
	"testing"
	"time"

	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/trigger"
)

// ADR-202: a run asked for by an earlier run (wake_at) goes at that time,
// even when only_if finds nothing; then the request is cleared and the
// schedule's own timing (and condition) resumes.
func TestWakeRunsOnceThenResumes(t *testing.T) {
	ctx := context.Background()
	st, p := openStore(t)
	ex := &fakeExec{}
	r := trigger.New(st, ex)
	ag, _ := st.Agents().Create(ctx, storage.Agent{ProjectID: p.ID, Key: "ceo", Name: "CEO", ModelTier: "fast"})
	a, err := st.Automations().Create(ctx, storage.Automation{ProjectID: p.ID, Name: "Điều phối", Source: "schedule", Action: "chat", Enabled: true, AgentID: ag.ID, Prompt: "x",
		Config: storage.AutomationConfig{EveryMinutes: 60, OnlyIf: &storage.AutomationCondition{Board: storage.BoardAny}}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	wake := now.Add(-time.Minute)
	if err := st.Automations().SetWake(ctx, a.ID, &wake); err != nil {
		t.Fatal(err)
	}
	r.Tick(ctx, now)
	r.Wait()
	if len(ex.chats) != 1 {
		t.Fatalf("the run asked for did not go (only_if had nothing): %v", ex.chats)
	}
	got, _ := st.Automations().Get(ctx, a.ID)
	if got.Config.WakeAt != nil || got.NextRunAt == nil || got.NextRunAt.Sub(now) < 59*time.Minute {
		t.Fatalf("after the woken run: wake %v next %v", got.Config.WakeAt, got.NextRunAt)
	}
	// its own beat again: only_if decides (the board is empty)
	later := got.NextRunAt.Add(time.Second)
	r.Tick(ctx, later)
	r.Wait()
	if len(ex.chats) != 1 {
		t.Fatalf("ran with nothing to do after the woken run: %v", ex.chats)
	}
}
