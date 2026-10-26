package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tung1998/agent-office/internal/actions"
	"github.com/tung1998/agent-office/internal/actor"
	"github.com/tung1998/agent-office/internal/audit"
	"github.com/tung1998/agent-office/internal/chat"
	"github.com/tung1998/agent-office/internal/officetools"
	"github.com/tung1998/agent-office/internal/storage"
)

// chatDecider decides proposals from a bot's chat (ADR-054) as the dashboard
// does: an approved one is the agent's change, approved by the person in the chat.
type chatDecider struct {
	store storage.Store
	chat  *chat.Engine
	acts  *actions.Service
}

func (d chatDecider) Decide(ctx context.Context, kind, id string, approve bool, by string) (string, error) {
	via, _, _ := strings.Cut(by, ":")
	if kind == "patch" {
		p, err := d.store.Chat().GetPatch(ctx, id)
		if err != nil {
			return "", err
		}
		if approve && p.Status == "pending" { // in the order its agent proposed them
			var order *chat.OrderError
			if err := d.chat.CheckOrder(ctx, p.ConversationID, p.ID); errors.As(err, &order) {
				if _, err := d.chat.QueuePatch(actor.With(ctx, by), p.ID); err != nil {
					return "", err
				}
				go d.chat.Drain(p.ConversationID)
				return fmt.Sprintf("đã xếp hàng, chạy sau #%d", order.Seq), nil
			} else if err != nil {
				return "", err
			}
		}
		project, agent := d.placeOf(ctx, p.ConversationID)
		who := audit.Who{Kind: "agent", Name: agent, ApprovedBy: by, Via: via, ConversationID: p.ConversationID}
		if !approve {
			who = audit.Who{Kind: "human", Name: by, Via: via, ConversationID: p.ConversationID}
		}
		ctx = audit.With(ctx, who)
		dto, err := d.chat.DecidePatch(ctx, id, approve)
		if err != nil {
			return "", err
		}
		verb := "patch.reject"
		if approve {
			verb = "patch." + dto.Status
		}
		var failed error
		if dto.Status == "failed" {
			failed = errors.New(dto.Detail)
		}
		_ = audit.Record(ctx, d.store.Audit(), audit.Change{Action: verb, ResourceID: id, ProjectID: project, Detail: map[string]any{"files": dto.Files, "detail": dto.Detail}, Err: failed})
		if failed != nil {
			return "", failed
		}
		return strings.Join(dto.Files, ", "), nil
	}
	cur, err := d.store.Actions().Get(ctx, id)
	if err != nil {
		return "", err
	}
	if approve && cur.Status == "pending" {
		var order *chat.OrderError
		if err := d.chat.CheckOrder(ctx, cur.ConversationID, cur.ID); errors.As(err, &order) {
			if _, err := d.acts.Queue(ctx, cur.ID, by); err != nil {
				return "", err
			}
			go d.chat.Drain(cur.ConversationID)
			return fmt.Sprintf("đã xếp hàng, chạy sau #%d", order.Seq), nil
		} else if err != nil {
			return "", err
		}
	}
	who := audit.Who{Kind: "agent", Name: cur.ProposedBy, ApprovedBy: by, Via: via, ConversationID: cur.ConversationID, JobID: cur.JobID, TaskID: cur.TaskID, ActionID: cur.ID}
	if !approve {
		who = audit.Who{Kind: "human", Name: by, Via: via, ConversationID: cur.ConversationID, ActionID: cur.ID}
	}
	ctx = audit.With(ctx, who)
	a, err := d.acts.Decide(ctx, id, approve, by)
	if err != nil {
		return "", err
	}
	verb := "action.reject"
	if approve {
		verb = "action.approve"
	}
	var runErr error
	if a.Status == "failed" {
		runErr = errors.New(a.Detail)
	}
	_ = audit.Record(ctx, d.store.Audit(), audit.Change{Action: verb, Resource: "action", ResourceID: a.ID, ProjectID: a.ProjectID,
		Detail: map[string]any{"kind": a.Kind, "target": a.Target, "status": a.Status}, Err: runErr})
	if runErr != nil {
		return "", runErr
	}
	return a.Detail, nil
}

// DecideAlways approves a command and lets its agent run the like of it on
// its own from now on ("♾️ Luôn cho phép").
func (d chatDecider) DecideAlways(ctx context.Context, id, by string) (string, error) {
	cur, err := d.store.Actions().Get(ctx, id)
	if err != nil {
		return "", err
	}
	if cur.Status == "pending" {
		if err := d.chat.CheckOrder(ctx, cur.ConversationID, cur.ID); err != nil {
			return "", err
		}
	}
	via, _, _ := strings.Cut(by, ":")
	ctx = audit.With(ctx, audit.Who{Kind: "agent", Name: cur.ProposedBy, ApprovedBy: by, Via: via, ConversationID: cur.ConversationID, JobID: cur.JobID, TaskID: cur.TaskID, ActionID: cur.ID})
	a, allowed, err := d.acts.DecideAlways(ctx, id, by)
	if err != nil { // refused before anything ran
		return "", err
	}
	var runErr error
	if a.Status == "failed" {
		runErr = errors.New(a.Detail)
	}
	_ = audit.Record(ctx, d.store.Audit(), audit.Change{Action: "action.approve", Resource: "action", ResourceID: a.ID, ProjectID: a.ProjectID,
		Detail: map[string]any{"kind": a.Kind, "target": a.Target, "status": a.Status, "always": allowed.Pattern}, Err: runErr})
	if runErr != nil {
		return "", runErr
	}
	return a.Detail + "\n" + actions.AlwaysNote(allowed), nil
}

// placeOf: a conversation's project and agent.
func (d chatDecider) placeOf(ctx context.Context, conversationID string) (string, string) {
	c, err := d.store.Chat().GetConversation(ctx, conversationID)
	if err != nil {
		return "", ""
	}
	return c.ProjectID, c.AgentName
}

// RunQueued carries out a queued card (chat.Drain) as approved by by, and
// says how it went, for its agent.
func (d chatDecider) RunQueued(ctx context.Context, kind, id, by string) string {
	label := id
	if kind == "patch" {
		ctx = actor.With(ctx, by)
		if p, err := d.store.Chat().GetPatch(ctx, id); err == nil {
			label = "diff: " + strings.Join(p.Files, ", ")
		}
	} else if a, err := d.store.Actions().Get(ctx, id); err == nil {
		label = cmp.Or(actions.Kinds[a.Kind], a.Kind) + ": " + a.Target
	}
	detail, err := d.Decide(ctx, kind, id, true, by)
	if err != nil {
		return "⚠️ Đã duyệt (xếp hàng) " + label + " — " + err.Error()
	}
	return "✅ Đã duyệt (xếp hàng) " + label + " — " + detail
}

// AdminDecide decides a waiting card for an administrator's run (ADR-188):
// of its own project (any, for the office assistant), never a push to
// master/main. The card's agent goes on as after a person's decision.
func (d chatDecider) AdminDecide(ctx context.Context, sc officetools.Scope, kind, id string, approve bool) (string, error) {
	var conv, project, label string
	switch kind {
	case "patch":
		p, err := d.store.Chat().GetPatch(ctx, id)
		if err != nil {
			return "", err
		}
		conv, label = p.ConversationID, "diff: "+strings.Join(p.Files, ", ")
		project, _ = d.placeOf(ctx, conv)
	case "action":
		a, err := d.store.Actions().Get(ctx, id)
		if err != nil {
			return "", err
		}
		if approve && a.Kind == "git_push" && d.acts.PushesProtected(ctx, a) {
			return "", errors.New("push lên master/main phải do người duyệt")
		}
		conv, project, label = a.ConversationID, a.ProjectID, cmp.Or(actions.Kinds[a.Kind], a.Kind)+": "+a.Target
	default:
		return "", errors.New("kind là action hoặc patch")
	}
	if !sc.Office && project != sc.ProjectID {
		return "", errors.New("thẻ này không thuộc project của bạn")
	}
	by := "auto:" + sc.Agent + " (administrator)"
	if sc.AdminBy != "" {
		ctx = actions.WithPrincipal(ctx, sc.AdminBy)
	}
	detail, err := d.Decide(ctx, kind, id, approve, by)
	if err != nil {
		return "", err
	}
	line := "❌ Đã từ chối " + label
	if approve {
		line = "✅ Đã duyệt " + label + " — " + detail
	}
	if conv != "" && conv != sc.ConversationID { // its agent goes on; one's own card it knows already
		d.chat.Decided(conv, by, line)
		go d.chat.Drain(conv)
	}
	return line, nil
}
