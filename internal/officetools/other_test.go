package officetools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tung1998/agent-office/internal/peer"
	"github.com/tung1998/agent-office/internal/perm"
	"github.com/tung1998/agent-office/internal/secrets"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/storage/sqlite"
)

// An office older than ADR-169 ignores the project a chat is asked as: a
// project's agent reads or sends into a chat it reaches only by its link's
// project once the other office says the chat is of it.
func TestOtherOldOffice(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, _ := sqlite.Open(filepath.Join(dir, "o.db"))
	defer st.Close()
	st.Migrate(ctx)
	box, _ := secrets.Load(filepath.Join(dir, "k"))
	project := "" // what the other office says of the chat ("" = an old office)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"text": "nội dung blog", "project": project})
	}))
	defer srv.Close()
	key, _ := box.Seal("ofp_x")
	st.Settings().Set(ctx, "peers", []peer.Peer{{ID: "peer_b", Name: "office-b", URL: srv.URL, OutKeyEnc: key,
		Remote: []peer.RemoteProject{{ID: "rep_shop", Name: "shop"}, {ID: "rep_blog", Name: "blog"}}}})
	p, _ := st.Repos().Create(ctx, storage.Repo{Name: "mine"})
	perm.SavePolicy(ctx, st, p.ID, perm.Policy{LinkedProjects: []string{"peer:peer_b/rep_shop"}})
	box2 := New(st, nil, nil)
	box2.SetPeers(peer.New(st, box))
	sc := Scope{ProjectID: p.ID, Level: perm.Operate}
	read := func(args string) (string, bool) { return box2.CallOther(ctx, sc, "read_chat", json.RawMessage(args)) }

	link := srv.URL + "/projects/rep_shop?tab=chat&c=cnv_blog"
	if out, isErr := read(`{"chat":"` + link + `"}`); !isErr || !strings.Contains(out, "không xác nhận") {
		t.Fatalf("an old office's chat read on the link's word: %v %s", isErr, out)
	}
	project = "rep_blog" // a new office: the chat is blog's
	if _, isErr := read(`{"chat":"` + link + `"}`); !isErr {
		t.Fatal("a blog chat read as shop's")
	}
	project = "rep_shop"
	if out, isErr := read(`{"chat":"` + link + `"}`); isErr || !strings.Contains(out, "nội dung blog") {
		t.Fatalf("shop's chat: %v %s", isErr, out)
	}
	// by office + project (name) + id, as the tool says; an id alone asks for the project
	if out, isErr := read(`{"office":"office-b","project":"shop","chat":"cnv_blog"}`); isErr {
		t.Fatalf("by id with project: %s", out)
	}
	if out, isErr := read(`{"office":"office-b","chat":"cnv_blog"}`); !isErr || !strings.Contains(out, "project") {
		t.Fatalf("by id alone: %v %s", isErr, out)
	}
	if _, isErr := read(`{"office":"office-b","project":"blog","chat":"cnv_blog"}`); !isErr {
		t.Fatal("an unlinked project's chat by id")
	}
}

// A chat another office handed work to answers back there (a result that
// came later) with no project of that office shared or linked: only that
// chat, and not on a report back alone.
func TestOtherAnswersHandOff(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, _ := sqlite.Open(filepath.Join(dir, "o.db"))
	defer st.Close()
	st.Migrate(ctx)
	box, _ := secrets.Load(filepath.Join(dir, "k"))
	key, _ := box.Seal("ofp_x")
	st.Settings().Set(ctx, "peers", []peer.Peer{{ID: "peer_b", Name: "office-b", URL: "https://b.example", OutKeyEnc: key}})
	p, _ := st.Repos().Create(ctx, storage.Repo{Name: "mine"})
	conv, _ := st.Chat().CreateConversation(ctx, storage.Conversation{ProjectID: p.ID})
	box2 := New(st, nil, nil)
	box2.SetPeers(peer.New(st, box))
	sc := Scope{ProjectID: p.ID, ConversationID: conv.ID, Level: perm.Operate}

	st.Chat().AddMessage(ctx, storage.Message{ConversationID: conv.ID, Role: "user", Content: "↩ Báo lại",
		Context: `{"relayed_from":"cnv_report","relayed_from_peer":"peer_b","relayed_from_project_id":"rep_there","relay_report":true}`})
	if box2.HasOther(ctx, sc, "send") {
		t.Fatal("a report back alone opened send")
	}
	st.Chat().AddMessage(ctx, storage.Message{ConversationID: conv.ID, Role: "user", Content: "làm trang shop",
		Context: `{"relayed_from":"cnv_there","relayed_from_peer":"peer_b","relayed_from_project_id":"rep_there"}`})
	if !box2.HasOther(ctx, sc, "send") || !box2.HasOther(ctx, sc, "read_chat") {
		t.Fatal("the chat that got work has no send back")
	}
	_, project, id, _, verify, err := box2.otherChat(ctx, sc, nil, "", "", "https://b.example/projects/rep_there?tab=chat&c=cnv_there")
	if err != nil || project != "rep_there" || id != "cnv_there" || verify {
		t.Fatalf("answer back: %q %q %v %v", project, id, verify, err)
	}
	for _, chat := range []string{"cnv_report", "cnv_other"} {
		if _, _, _, _, _, err := box2.otherChat(ctx, sc, nil, "office-b", "", chat); err == nil {
			t.Fatalf("%s reached with no project", chat)
		}
	}
}
