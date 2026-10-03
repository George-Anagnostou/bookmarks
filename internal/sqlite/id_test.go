package sqlite

import "testing"

func TestNewID(t *testing.T) {
	id, err := newID()
	if err != nil {
		t.Fatalf("newID() error = %v", err)
	}
	if len(id) != 32 {
		t.Fatalf("newID() length = %d, want 32", len(id))
	}

	seen := map[string]bool{id: true}
	for range 100 {
		id, err := newID()
		if err != nil {
			t.Fatalf("newID() error = %v", err)
		}
		if seen[id] {
			t.Fatalf("newID() returned duplicate id %q", id)
		}
		seen[id] = true
	}
}
