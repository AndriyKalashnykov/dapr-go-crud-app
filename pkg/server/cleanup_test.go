package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"
)

// fakeStorage is a thread-safe in-memory TodosStorage with knobs for
// programming Delete to fail. Lets cleanupLoop tests assert each
// branch (success, ListAll error, Delete error) without time.Sleep.
type fakeStorage struct {
	mu        sync.Mutex
	items     []*todos.Todo
	listErr   error
	deleteErr error
	deletes   int
}

func (f *fakeStorage) Create(t *todos.Todo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = append(f.items, t)
	return nil
}

func (f *fakeStorage) Update(*todos.Todo) error { return nil }

func (f *fakeStorage) Delete(t *todos.Todo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletes++
	out := f.items[:0]
	for _, x := range f.items {
		if x != t {
			out = append(out, x)
		}
	}
	f.items = out
	return nil
}

func (f *fakeStorage) ListAll() ([]*todos.Todo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*todos.Todo, len(f.items))
	copy(out, f.items)
	return out, nil
}

func TestCleanupLoop_DeletesEveryItemOnTick(t *testing.T) {
	fs := &fakeStorage{items: []*todos.Todo{{Id: "1"}, {Id: "2"}, {Id: "3"}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tick := make(chan time.Time, 1)
	done := make(chan struct{})
	go func() { cleanupLoop(ctx, fs, tick); close(done) }()

	tick <- time.Now()

	// Wait for the goroutine to drain and apply the deletes.
	if waitFor(t, time.Second, func() bool {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return fs.deletes == 3 && len(fs.items) == 0
	}) {
		t.Fatalf("cleanupLoop did not delete all items in time (deletes=%d, remaining=%d)", fs.deletes, len(fs.items))
	}

	cancel()
	<-done
}

func TestCleanupLoop_StopsOnContextCancel(t *testing.T) {
	fs := &fakeStorage{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() { cleanupLoop(ctx, fs, make(chan time.Time)); close(done) }()

	cancel()

	select {
	case <-done:
		// expected
	case <-time.After(time.Second):
		t.Fatal("cleanupLoop did not return within 1s of ctx cancel")
	}
}

func TestCleanupLoop_ContinuesAfterListAllError(t *testing.T) {
	fs := &fakeStorage{listErr: errors.New("boom")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tick := make(chan time.Time, 2)
	done := make(chan struct{})
	go func() { cleanupLoop(ctx, fs, tick); close(done) }()

	tick <- time.Now() // ListAll fails — loop continues
	// Clear the error and tick again; the loop must still be running.
	fs.mu.Lock()
	fs.listErr = nil
	fs.items = []*todos.Todo{{Id: "x"}}
	fs.mu.Unlock()
	tick <- time.Now()

	if waitFor(t, time.Second, func() bool {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return fs.deletes == 1
	}) {
		t.Fatalf("loop did not recover from ListAll error (deletes=%d)", fs.deletes)
	}

	cancel()
	<-done
}

func TestGenerateLoadLoop_CreatesItemOnTick(t *testing.T) {
	fs := &fakeStorage{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tick := make(chan time.Time, 1)
	done := make(chan struct{})
	go func() { generateLoadLoop(ctx, fs, tick); close(done) }()

	tick <- time.Now()

	if waitFor(t, time.Second, func() bool {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return len(fs.items) == 1 && fs.items[0].Text == "foo"
	}) {
		t.Fatalf("generateLoadLoop did not create the synthetic todo (items=%d)", len(fs.items))
	}

	cancel()
	<-done
}

// waitFor returns true if cond never became true within d. (Inverted return
// to match `if waitFor(...) t.Fatal` in callers.)
func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return true
}
