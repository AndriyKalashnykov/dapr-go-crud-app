//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/storage"
	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
)

// Pinned to match the version Renovate tracks via the Makefile MONGO_VERSION
// constant — keep them in lockstep so the test environment matches `make
// mongo-run` and the deploy/redis-Helm-equivalent local Mongo flow.
const mongoImage = "mongo:8.0"

// startMongo spins up a fresh MongoDB Testcontainer and returns a connected
// `*mongo.Collection` against an isolated database/collection that auto-
// dies with the container. Each test gets its own collection so they can
// run in parallel without bleeding state.
func startMongo(t *testing.T) *mongo.Collection {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	container, err := tcmongo.Run(ctx, mongoImage)
	if err != nil {
		t.Fatalf("start mongo container: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate mongo container: %v", err)
		}
	})

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("mongo.Connect: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	})
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("mongo Ping: %v", err)
	}

	return client.Database("mgm_lab_test").Collection(t.Name())
}

func TestMongoStorage_CreateThenListAll_RoundTrip(t *testing.T) {
	coll := startMongo(t)
	s := storage.NewMongoStorageFromCollection(coll, 10)

	td := &todos.Todo{Text: "buy milk"}
	if err := s.Create(td); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if td.Id == "" {
		t.Fatal("Create did not assign Id")
	}
	if td.CreatedAt.IsZero() || td.UpdatedAt.IsZero() {
		t.Errorf("Create did not stamp CreatedAt/UpdatedAt: %+v", td)
	}

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 || all[0].Id != td.Id || all[0].Text != "buy milk" {
		t.Fatalf("ListAll = %+v, want single round-tripped todo", all)
	}

	// Pin the field-name mapping (`todoId` not `id`) — a future BSON tag
	// change would silently break Update/Delete which filter on `todoId`.
	var raw bson.M
	if err := coll.FindOne(context.Background(), bson.M{"todoId": td.Id}).Decode(&raw); err != nil {
		t.Errorf("FindOne by todoId: %v (the BSON tag is `todoId`, not `id`)", err)
	}
}

func TestMongoStorage_Update_PersistsChanges(t *testing.T) {
	coll := startMongo(t)
	s := storage.NewMongoStorageFromCollection(coll, 10)

	td := &todos.Todo{Text: "first"}
	if err := s.Create(td); err != nil {
		t.Fatalf("Create: %v", err)
	}
	beforeUpdate := td.UpdatedAt

	td.Text = "edited"
	td.Done = "true"
	time.Sleep(2 * time.Millisecond) // stamp must monotonically increase
	if err := s.Update(td); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !td.UpdatedAt.After(beforeUpdate) {
		t.Errorf("Update did not refresh UpdatedAt (before=%v, after=%v)", beforeUpdate, td.UpdatedAt)
	}

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 || all[0].Text != "edited" || all[0].Done != "true" {
		t.Fatalf("ListAll after Update = %+v, want Text='edited' Done='true'", all)
	}
}

func TestMongoStorage_Delete_RemovesTodo(t *testing.T) {
	coll := startMongo(t)
	s := storage.NewMongoStorageFromCollection(coll, 10)

	td := &todos.Todo{Text: "ephemeral"}
	if err := s.Create(td); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Delete(td); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("ListAll after Delete = %+v, want empty", all)
	}
}

func TestMongoStorage_Create_EvictsOldestAtMaxItems(t *testing.T) {
	const max = 3
	coll := startMongo(t)
	s := storage.NewMongoStorageFromCollection(coll, max)

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
		t.Fatalf("ListAll len = %d, want %d (cap)", len(all), max)
	}
}
