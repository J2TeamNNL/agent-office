package main

import (
	"testing"

	"github.com/tung1998/agent-office/internal/memory"
)

// ADR-215: what a review model answers becomes changes to the notes; other
// lines are ignored.
func TestParseReview(t *testing.T) {
	got := parseReview("Here you go:\n- ADD deploy | build, release | build bằng `make dist`, không dùng go build\nADD - | - | repo là bản private\nEDIT mem_1 | chạy pnpm test | trước khi báo xong\nDROP mem_2 (cũ)\nNONE\nADD thiếu")
	want := []memory.Op{
		{Op: "add", Topic: "deploy", Summary: "build, release", Text: "build bằng `make dist`, không dùng go build"},
		{Op: "add", Text: "repo là bản private"},
		{Op: "edit", ID: "mem_1", Text: "chạy pnpm test | trước khi báo xong"},
		{Op: "drop", ID: "mem_2"},
	}
	if len(got) != len(want) {
		t.Fatalf("ops = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("op %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
