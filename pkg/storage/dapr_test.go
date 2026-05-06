package storage

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"
	dapr "github.com/dapr/go-sdk/client"
)

// fakeDaprClient is an in-memory daprStateClient used by DaprStorage
// unit tests. Records every call (state writes + published events) so
// assertions can inspect post-conditions; programmable per-method
// failure via failNext for error-path coverage.
type fakeDaprClient struct {
	mu              sync.Mutex
	state           map[string][]byte
	publishedEvents []publishedEvent
	failNext        map[string]error
}

type publishedEvent struct {
	pubsub string
	topic  string
}

func newFakeDaprClient() *fakeDaprClient {
	return &fakeDaprClient{
		state:    map[string][]byte{},
		failNext: map[string]error{},
	}
}

func (f *fakeDaprClient) checkFail(method string) error {
	if err, ok := f.failNext[method]; ok {
		delete(f.failNext, method)
		return err
	}
	return nil
}

func (f *fakeDaprClient) SaveBulkState(_ context.Context, _ string, items ...*dapr.SetStateItem) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkFail("SaveBulkState"); err != nil {
		return err
	}
	for _, item := range items {
		f.state[item.Key] = item.Value
	}
	return nil
}

func (f *fakeDaprClient) GetState(_ context.Context, _, key string, _ map[string]string) (*dapr.StateItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkFail("GetState"); err != nil {
		return nil, err
	}
	v, ok := f.state[key]
	if !ok {
		// Real Dapr returns a StateItem with nil Value for missing keys.
		return &dapr.StateItem{Key: key}, nil
	}
	return &dapr.StateItem{Key: key, Value: v}, nil
}

func (f *fakeDaprClient) SaveState(_ context.Context, _, key string, data []byte, _ map[string]string, _ ...dapr.StateOption) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkFail("SaveState"); err != nil {
		return err
	}
	f.state[key] = data
	return nil
}

func (f *fakeDaprClient) DeleteState(_ context.Context, _, key string, _ map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkFail("DeleteState"); err != nil {
		return err
	}
	delete(f.state, key)
	return nil
}

func (f *fakeDaprClient) GetBulkState(_ context.Context, _ string, keys []string, _ map[string]string, _ int32) ([]*dapr.BulkStateItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkFail("GetBulkState"); err != nil {
		return nil, err
	}
	out := make([]*dapr.BulkStateItem, 0, len(keys))
	for _, k := range keys {
		if v, ok := f.state[k]; ok {
			out = append(out, &dapr.BulkStateItem{Key: k, Value: v})
		}
	}
	return out, nil
}

func (f *fakeDaprClient) PublishEvent(_ context.Context, pubsub, topic string, _ any, _ ...dapr.PublishEventOption) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkFail("PublishEvent"); err != nil {
		return err
	}
	f.publishedEvents = append(f.publishedEvents, publishedEvent{pubsub: pubsub, topic: topic})
	return nil
}

// Compile-time assertion that fakeDaprClient satisfies daprStateClient.
var _ daprStateClient = (*fakeDaprClient)(nil)

func TestDaprStorage_Create_AssignsId_PersistsState_PublishesEvent(t *testing.T) {
	fc := newFakeDaprClient()
	s := NewDaprStorageWithClient(fc, 10)

	td := &todos.Todo{Text: "buy milk"}
	if err := s.Create(td); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if td.Id == "" {
		t.Fatal("Create did not assign Id")
	}

	saved, ok := fc.state[td.Id]
	if !ok {
		t.Fatalf("state[%q] not persisted", td.Id)
	}
	var roundtrip todos.Todo
	if err := json.Unmarshal(saved, &roundtrip); err != nil {
		t.Fatalf("unmarshal saved value: %v", err)
	}
	if roundtrip.Text != "buy milk" {
		t.Errorf("roundtripped Text = %q, want %q", roundtrip.Text, "buy milk")
	}

	// Index updated.
	var index []string
	if err := json.Unmarshal(fc.state[indexKey], &index); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	if len(index) != 1 || index[0] != td.Id {
		t.Errorf("index = %v, want [%q]", index, td.Id)
	}

	if len(fc.publishedEvents) != 1 {
		t.Fatalf("PublishEvent fired %d times, want 1", len(fc.publishedEvents))
	}
	if fc.publishedEvents[0].pubsub != pubsubName || fc.publishedEvents[0].topic != pubsubTopic {
		t.Errorf("publish target = %q/%q, want %q/%q",
			fc.publishedEvents[0].pubsub, fc.publishedEvents[0].topic, pubsubName, pubsubTopic)
	}
}

func TestDaprStorage_Create_FIFOEvictsAtMaxItems(t *testing.T) {
	const max = 3
	fc := newFakeDaprClient()
	s := NewDaprStorageWithClient(fc, max)

	ids := []string{}
	for _, text := range []string{"a", "b", "c", "d", "e"} {
		td := &todos.Todo{Text: text}
		if err := s.Create(td); err != nil {
			t.Fatalf("Create(%s): %v", text, err)
		}
		ids = append(ids, td.Id)
	}

	var index []string
	if err := json.Unmarshal(fc.state[indexKey], &index); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	if len(index) != max {
		t.Fatalf("index len = %d, want %d (FIFO cap)", len(index), max)
	}

	// FIFO: first 2 evicted; last 3 retained in insertion order.
	wantIDs := ids[2:5]
	for i, id := range index {
		if id != wantIDs[i] {
			t.Errorf("index[%d] = %s, want %s (insertion order)", i, id, wantIDs[i])
		}
	}
}

func TestDaprStorage_Update_ReplacesValue_PublishesEvent(t *testing.T) {
	fc := newFakeDaprClient()
	s := NewDaprStorageWithClient(fc, 10)

	td := &todos.Todo{Text: "v1"}
	if err := s.Create(td); err != nil {
		t.Fatalf("Create: %v", err)
	}

	td.Text = "v2"
	td.Done = "true"
	if err := s.Update(td); err != nil {
		t.Fatalf("Update: %v", err)
	}

	var roundtrip todos.Todo
	if err := json.Unmarshal(fc.state[td.Id], &roundtrip); err != nil {
		t.Fatalf("unmarshal after Update: %v", err)
	}
	if roundtrip.Text != "v2" || roundtrip.Done != "true" {
		t.Errorf("after Update: Text=%q Done=%q, want v2/true", roundtrip.Text, roundtrip.Done)
	}

	if len(fc.publishedEvents) != 2 {
		t.Errorf("PublishEvent fired %d times, want 2 (Create + Update)", len(fc.publishedEvents))
	}
}

func TestDaprStorage_Delete_RemovesFromState_AndIndex(t *testing.T) {
	fc := newFakeDaprClient()
	s := NewDaprStorageWithClient(fc, 10)

	td := &todos.Todo{Text: "ephemeral"}
	if err := s.Create(td); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(td); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, ok := fc.state[td.Id]; ok {
		t.Errorf("state[%q] still present after Delete", td.Id)
	}

	var index []string
	if err := json.Unmarshal(fc.state[indexKey], &index); err != nil {
		t.Fatalf("unmarshal index after Delete: %v", err)
	}
	if len(index) != 0 {
		t.Errorf("index after Delete = %v, want []", index)
	}

	if len(fc.publishedEvents) != 2 {
		t.Errorf("PublishEvent fired %d times, want 2 (Create + Delete)", len(fc.publishedEvents))
	}
}

func TestDaprStorage_ListAll_EmptyIndexReturnsEmpty(t *testing.T) {
	fc := newFakeDaprClient()
	s := NewDaprStorageWithClient(fc, 10)

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("ListAll on empty = %d items, want 0", len(all))
	}
}

func TestDaprStorage_ListAll_RoundTripsValues(t *testing.T) {
	fc := newFakeDaprClient()
	s := NewDaprStorageWithClient(fc, 10)

	for _, text := range []string{"a", "b", "c"} {
		if err := s.Create(&todos.Todo{Text: text}); err != nil {
			t.Fatalf("Create(%s): %v", text, err)
		}
	}

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListAll = %d items, want 3", len(all))
	}
	got := []string{all[0].Text, all[1].Text, all[2].Text}
	want := []string{"a", "b", "c"}
	for i, text := range got {
		if text != want[i] {
			t.Errorf("ListAll[%d].Text = %q, want %q", i, text, want[i])
		}
	}
}

func TestDaprStorage_Create_PropagatesIndexFetchError(t *testing.T) {
	fc := newFakeDaprClient()
	fc.failNext["GetState"] = errors.New("upstream sidecar timeout")
	s := NewDaprStorageWithClient(fc, 10)

	err := s.Create(&todos.Todo{Text: "x"})
	if err == nil {
		t.Fatal("Create succeeded despite GetState failure")
	}
	if len(fc.publishedEvents) != 0 {
		t.Errorf("PublishEvent fired %d times after upstream failure, want 0", len(fc.publishedEvents))
	}
}

func TestDaprStorage_Implements_TodosStorage(t *testing.T) {
	// Compile-time check that DaprStorage satisfies the storage interface.
	var _ TodosStorage = (*DaprStorage)(nil)
}
