package storage

import (
	"testing"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"
)

func TestInMemoryStorage_Create_AssignsID(t *testing.T) {
	s := NewInMemoryStorage(10)
	td := &todos.Todo{Text: "buy milk"}

	if err := s.Create(td); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if td.Id == "" {
		t.Fatal("Create did not assign Id")
	}

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 || all[0].Id != td.Id || all[0].Text != "buy milk" {
		t.Fatalf("ListAll = %+v, want single todo with assigned id and matching text", all)
	}
}

func TestInMemoryStorage_Create_FIFOEvictionAtMax(t *testing.T) {
	const max = 3
	s := NewInMemoryStorage(max)

	for _, text := range []string{"a", "b", "c", "d", "e"} {
		if err := s.Create(&todos.Todo{Text: text}); err != nil {
			t.Fatalf("Create(%s): %v", text, err)
		}
	}

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != max {
		t.Fatalf("ListAll len = %d, want %d (FIFO cap)", len(all), max)
	}

	// FIFO: a, b should be evicted; c, d, e remain in insertion order.
	wantTexts := []string{"c", "d", "e"}
	for i, td := range all {
		if td.Text != wantTexts[i] {
			t.Errorf("all[%d].Text = %q, want %q (insertion order preserved)", i, td.Text, wantTexts[i])
		}
	}

	// Distinct IDs (regression — would catch a future Create that reuses IDs).
	seen := map[string]bool{}
	for _, td := range all {
		if seen[td.Id] {
			t.Errorf("duplicate Id %q in ListAll", td.Id)
		}
		seen[td.Id] = true
	}
}

func TestInMemoryStorage_UpdateAndDelete_AreNoOp(t *testing.T) {
	// Documented behaviour of the in-mem backend: Update/Delete are no-ops
	// (they're handled at the Dapr/Mongo backends, not in-memory). Pin this
	// shape so a future "fix" doesn't silently drop todos in dev runs that
	// rely on -connStr=mem.
	s := NewInMemoryStorage(10)
	td := &todos.Todo{Text: "x"}
	if err := s.Create(td); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Update(td); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := s.Delete(td); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListAll after Update+Delete len = %d, want 1 (both are no-ops)", len(all))
	}
}

func TestInMemoryStorage_ImplementsTodosStorageInterface(t *testing.T) {
	// Compile-time assertion — interface drift would break builds, but the
	// explicit assignment makes the contract visible at the test level.
	var _ TodosStorage = NewInMemoryStorage(1)
}
