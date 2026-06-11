package bookmarks

import "testing"

func TestNewID(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	if len(id) != 32 {
		t.Fatalf("NewID() length = %d, want 32", len(id))
	}

	seen := map[string]bool{id: true}
	for range 100 {
		id, err := NewID()
		if err != nil {
			t.Fatalf("NewID() error = %v", err)
		}
		if seen[id] {
			t.Fatalf("NewID() returned duplicate id %q", id)
		}
		seen[id] = true
	}
}
