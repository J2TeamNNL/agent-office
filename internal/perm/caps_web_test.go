package perm

import (
	"slices"
	"testing"

	"github.com/tung1998/agent-office/internal/storage"
)

// ADR-192: tools.web is in no package; an agent has it when picked, and an
// administrator's run has every capability.
func TestWebIsOptIn(t *testing.T) {
	for _, l := range []string{Read, Propose, Operate} {
		if slices.Contains(Preset(l), CapWeb) {
			t.Fatalf("%s has web by default", l)
		}
	}
	picked := []string{CapWeb}
	a := storage.Agent{Permissions: storage.Permissions{Level: Read, Caps: &picked}}
	if !Resolve(a, "", DefaultPolicy()).Can(CapWeb) {
		t.Fatal("a picked web is not there")
	}
	if !Admin(Access{}).Can(CapWeb) {
		t.Fatal("an administrator has no web")
	}
}
