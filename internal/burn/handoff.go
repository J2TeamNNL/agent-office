package burn

import (
	"context"
	"log/slog"
	"strings"

	"github.com/tung1998/agent-office/internal/chat"
)

// OnBoard keeps a task of the work board in step with the hand-off it is
// (ADR-210): an agent given it with delegate (task_id) has it in progress
// (handed: the Burn leaves it alone), its answer makes it done with that
// answer as its summary, a hand-off that ends without one makes it failed.
// A task of another project's board, or one already finished, is left as it is.
func (s *Service) OnBoard(ctx context.Context, ev chat.BoardEvent) {
	it, err := s.store.Burn().Item(ctx, ev.Task)
	if err != nil {
		return
	}
	b, err := s.store.Burn().SessionByID(ctx, it.SessionID)
	if err != nil || b.ProjectID != ev.ProjectID {
		return
	}
	from := it.Status
	switch {
	case !ev.Done:
		if !openStatus[from] || from == "doing" { // the Burn is on it
			return
		}
		it.Status, it.Summary = StatusHanded, "→ "+ev.Agent
	case from != StatusHanded:
		return // moved on meanwhile (skipped by the person, taken by the Burn)
	case ev.Failed || strings.TrimSpace(ev.Result) == "":
		it.Status, it.Summary = "failed", ev.Agent+": "+clip(strings.TrimSpace(ev.Result), 1500)
	default:
		it.Status, it.Summary = "done", ev.Agent+": "+clip(strings.TrimSpace(ev.Result), 3000)
	}
	if err := s.store.Burn().UpdateItemFrom(ctx, it, from); err != nil {
		slog.Warn("burn: board task", "item", it.ID, "err", err)
		return
	}
	s.wake(b.ProjectID)
}
