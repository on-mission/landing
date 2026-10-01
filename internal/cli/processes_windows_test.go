//go:build windows

package cli

import (
	"context"
	"testing"
)

func TestProcessesReturnsNoParents(t *testing.T) {
	// Bug class: Windows messages stop because Unix ps is found and exits 1.
	entries, err := processes(context.Background())
	if err != nil {
		t.Fatalf("processes() returned unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("processes() = %#v, want no parent processes", entries)
	}
	chat, err := identifyChat(context.Background(), parsedOption{Value: "office", Set: true})
	if err != nil {
		t.Fatalf("identifyChat() returned unexpected error: %v", err)
	}
	if chat.name != "office" || chat.session != "office" || chat.harness != "" {
		t.Fatalf("chat = %+v; want name and session office with no harness", chat)
	}
}
