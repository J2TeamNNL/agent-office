package chat

import (
	"cmp"
	"context"
	"errors"
	"github.com/tung1998/agent-office/internal/prompts"
	"strings"
	"sync"

	"github.com/tung1998/agent-office/internal/actor"
	"github.com/tung1998/agent-office/internal/storage"
)

// StartConversationFor picks the agent of a new conversation without storing it.
func (e *Engine) StartConversationFor(ctx context.Context, projectID, agentID string) (storage.Conversation, error) {
	agents, err := e.Agents(ctx, projectID)
	if err != nil {
		return storage.Conversation{}, err
	}
	pick := func(a storage.Agent) storage.Conversation {
		return storage.Conversation{ProjectID: projectID, AgentID: a.ID, AgentName: a.Name, CreatedBy: actor.From(ctx)}
	}
	if agentID == "" { // the project's default agent
		a, err := e.defaultAgent(ctx, projectID, agents)
		return pick(a), err
	}
	for _, a := range agents {
		if a.ID == agentID { // even paused: its first message gets the notice
			return pick(a), nil
		}
	}
	return storage.Conversation{}, ErrNoAgent
}

var newChatMu sync.Mutex // one "new chat" at a time: clicks in a row get the same one

// NewConversation is "new chat" from the dashboard: the person's latest chat
// that is still empty is brought back to the top (with agentID) rather than
// another empty one made.
func (e *Engine) NewConversation(ctx context.Context, projectID, agentID string) (storage.Conversation, error) {
	c, err := e.StartConversationFor(ctx, projectID, agentID)
	if err != nil || !strings.HasPrefix(c.CreatedBy, "human:") {
		return c, err
	}
	newChatMu.Lock()
	defer newChatMu.Unlock()
	old, err := e.store.Chat().EmptyConversation(ctx, projectID, c.CreatedBy)
	if errors.Is(err, storage.ErrNotFound) {
		return e.store.Chat().CreateConversation(ctx, c)
	}
	if err != nil {
		return old, err
	}
	old.AgentID, old.AgentName = c.AgentID, c.AgentName
	if err := e.store.Chat().UpdateConversation(ctx, old); err != nil { // updated_at: on top again
		return old, err
	}
	return e.store.Chat().GetConversation(ctx, old.ID)
}

// StartConversationPurpose opens a thread for a purpose ("automation" = one
// that builds an automation, not listed with the project's chats).
func (e *Engine) StartConversationPurpose(ctx context.Context, projectID, agentID, purpose string) (storage.Conversation, error) {
	c, err := e.StartConversationFor(ctx, projectID, agentID)
	if err != nil {
		return c, err
	}
	c.Purpose = purpose
	return e.store.Chat().CreateConversation(ctx, c)
}

// Conversation is one chat as stored.
func (e *Engine) Conversation(ctx context.Context, id string) (storage.Conversation, error) {
	return e.store.Chat().GetConversation(ctx, id)
}

// ErrCleaned: the chat's data was cleaned up; it takes no more messages.
var ErrCleaned = errors.New("chat này đã được dọn dữ liệu nên không nhắn thêm được: hãy mở chat mới")

// AddTags puts tags on a chat beside its own ("Bug" = "bug"), at most 10 in all.
func (e *Engine) AddTags(ctx context.Context, conversationID string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	c, err := e.store.Chat().GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	all, seen := c.Tags, map[string]bool{}
	for _, t := range all {
		seen[strings.ToLower(t)] = true
	}
	for _, t := range tags {
		if !seen[strings.ToLower(t)] && len(all) < 10 {
			seen[strings.ToLower(t)] = true
			all = append(all, t)
		}
	}
	if len(all) == len(c.Tags) {
		return nil
	}
	return e.store.Chat().SetConversationTags(ctx, conversationID, all)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// RoleChange is what changed in an agent's job (ADR-194).
type RoleChange struct {
	By      string
	Changes []string
}

// Brief opens a chat with agent a and tells it its job changed, so it brings
// its notes and schedules up to date. It speaks for the person behind ctx; a
// change no person made (an import, office itself) tells no one.
func (e *Engine) Brief(ctx context.Context, a storage.Agent, ch RoleChange) error {
	as := actor.From(ctx)
	if !strings.HasPrefix(as, "human:") || len(ch.Changes) == 0 {
		return nil
	}
	ch.By = cmp.Or(ch.By, strings.TrimPrefix(as, "human:"))
	c, err := e.StartConversationFor(ctx, a.ProjectID, a.ID)
	if err != nil {
		return err
	}
	c.Title = "Cập nhật vai: " + a.Name // i18n-ignore
	if c, err = e.store.Chat().CreateConversation(ctx, c); err != nil {
		return err
	}
	_, _, err = e.SendWithContext(actor.With(context.Background(), as), c.ID, prompts.Render("chat/role-changed", ch), "", nil)
	var off *OffError
	if errors.As(err, &off) {
		return nil // a paused agent reads it when it is back
	}
	return err
}
