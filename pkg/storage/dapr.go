package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"
	dapr "github.com/dapr/go-sdk/client"
	"github.com/google/uuid"
)

// daprStateClient is the subset of dapr.Client that DaprStorage uses.
// Defined locally (not in the SDK) so unit tests inject a fake without
// implementing the full 50+-method dapr.Client interface — Pattern B
// from /test-coverage-analysis. The concrete `dapr.Client` returned by
// `dapr.NewClient()` is a superset and satisfies this interface
// implicitly.
type daprStateClient interface {
	SaveBulkState(ctx context.Context, store string, items ...*dapr.SetStateItem) error
	GetState(ctx context.Context, store, key string, meta map[string]string) (*dapr.StateItem, error)
	SaveState(ctx context.Context, store, key string, data []byte, meta map[string]string, so ...dapr.StateOption) error
	DeleteState(ctx context.Context, store, key string, meta map[string]string) error
	GetBulkState(ctx context.Context, store string, keys []string, meta map[string]string, parallelism int32) ([]*dapr.BulkStateItem, error)
	PublishEvent(ctx context.Context, pubsubName, topicName string, data any, opts ...dapr.PublishEventOption) error
}

type DaprStorage struct {
	client   daprStateClient
	maxItems int
}

var statestoreName string = "statestore"
var pubsubName string = "pubsub"
var pubsubTopic string = "todos"
var indexKey string = "index"

func (s *DaprStorage) Create(todo *todos.Todo) error {
	todo.Id = uuid.New().String()
	bytes, err := json.Marshal(todo)
	if err != nil {
		return err
	}

	allTodos, err := s.getIndexState()
	if err != nil {
		return err
	}

	if len(allTodos) >= s.maxItems {
		allTodos = allTodos[1:]
	}

	allTodos = append(allTodos, todo.Id)
	atb, err := json.Marshal(allTodos)
	if err != nil {
		return err
	}

	err = s.newDaprClient().SaveBulkState(context.Background(), statestoreName,
		&dapr.SetStateItem{Key: todo.Id, Value: bytes},
		&dapr.SetStateItem{Key: indexKey, Value: atb})
	if err != nil {
		return err
	}

	err = s.publishEvent(todo)
	if err != nil {
		return err
	}

	return nil
}

func (s *DaprStorage) getIndexState() ([]string, error) {
	index, err := s.newDaprClient().GetState(context.TODO(), statestoreName, indexKey, nil)
	if err != nil {
		return nil, fmt.Errorf("get index state from dapr: %w", err)
	}
	if index == nil || index.Value == nil {
		return make([]string, 0), nil
	}
	a := make([]string, 0)
	err = json.Unmarshal(index.Value, &a)
	if err != nil {
		return nil, fmt.Errorf("index json deserialization: %w", err)
	}
	return a, nil
}

func (s *DaprStorage) Update(todo *todos.Todo) error {
	bytes, err := json.Marshal(todo)
	if err != nil {
		return err
	}

	err = s.newDaprClient().SaveState(context.Background(), statestoreName, todo.Id, bytes, nil)
	if err != nil {
		return err
	}

	err = s.publishEvent(todo)
	if err != nil {
		return err
	}

	return nil
}

func (s *DaprStorage) Delete(todo *todos.Todo) error {
	allTodos, err := s.getIndexState()
	if err != nil {
		return err
	}

	newtodos := make([]string, 0)
	for _, t := range allTodos {
		if t != todo.Id {
			newtodos = append(newtodos, t)
		}
	}

	atb, err := json.Marshal(newtodos)
	if err != nil {
		return err
	}

	fmt.Printf("deleting todo: %v\n", todo.Id)
	err = s.newDaprClient().DeleteState(context.Background(), statestoreName, todo.Id, nil)
	if err != nil {
		return err
	}
	err = s.newDaprClient().SaveState(context.Background(), statestoreName, indexKey, atb, nil)
	if err != nil {
		return err
	}

	err = s.publishEvent(todo)
	if err != nil {
		return err
	}

	return nil
}

func (s *DaprStorage) publishEvent(t *todos.Todo) error {
	return s.newDaprClient().PublishEvent(context.Background(), pubsubName, pubsubTopic, t, dapr.PublishEventWithMetadata(map[string]string{"rawPayload": "true"}))
}

func (s *DaprStorage) ListAll() ([]*todos.Todo, error) {

	keys, err := s.getIndexState()
	if err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return make([]*todos.Todo, 0), nil
	}

	items, err := s.newDaprClient().GetBulkState(context.Background(), statestoreName, keys, nil, 1)
	if err != nil {
		return nil, err
	}
	all := []*todos.Todo{}

	for _, item := range items {
		t := todos.Todo{}
		if err := json.Unmarshal(item.Value, &t); err != nil {
			return nil, err
		}
		all = append(all, &t)
	}
	return all, nil
}

func (s *DaprStorage) newDaprClient() daprStateClient {
	if s.client == nil {
		client, err := dapr.NewClient()
		if err != nil {
			panic(err)
		}
		s.client = client
	}
	return s.client
}

// NewDaprStorage returns a DaprStorage with lazy Dapr-sidecar dialing.
// The client is created on first method call via `dapr.NewClient()`,
// which reads `DAPR_GRPC_PORT` / `DAPR_GRPC_ENDPOINT` env vars.
func NewDaprStorage(maxItems int) *DaprStorage {
	return &DaprStorage{maxItems: maxItems}
}

// NewDaprStorageWithClient wraps an injected daprStateClient. Tests
// pass an in-memory fake; production goes through NewDaprStorage which
// dials a real sidecar.
func NewDaprStorageWithClient(client daprStateClient, maxItems int) *DaprStorage {
	return &DaprStorage{client: client, maxItems: maxItems}
}
