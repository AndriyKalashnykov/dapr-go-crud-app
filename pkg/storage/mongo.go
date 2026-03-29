package storage

import (
	"context"
	"time"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoStorage struct {
	coll     *mongo.Collection
	maxItems int
}

var mongoImpl TodosStorage = &MongoStorage{}

func (s *MongoStorage) Create(todo *todos.Todo) error {
	todo.Id = uuid.New().String()
	todo.CreatedAt = time.Now()
	todo.UpdatedAt = time.Now()

	count, err := s.coll.CountDocuments(context.Background(), bson.D{})
	if err != nil {
		return err
	}
	if count >= int64(s.maxItems) {
		// set the sort to -1 to sort descending and get the last item for deletion
		deleteOptions := options.FindOneAndDelete().SetSort(bson.D{{Key: "_id", Value: -1}})
		s.coll.FindOneAndDelete(context.Background(), bson.D{}, deleteOptions)
	}
	_, err = s.coll.InsertOne(context.Background(), todo)
	return err
}

func (s *MongoStorage) Update(todo *todos.Todo) error {
	todo.UpdatedAt = time.Now()
	filter := bson.D{{Key: "todoId", Value: todo.Id}}
	update := bson.D{{Key: "$set", Value: todo}}
	_, err := s.coll.UpdateOne(context.Background(), filter, update)
	return err
}

func (s *MongoStorage) Delete(todo *todos.Todo) error {
	filter := bson.D{{Key: "todoId", Value: todo.Id}}
	_, err := s.coll.DeleteOne(context.Background(), filter)
	return err
}

func (s *MongoStorage) ListAll() ([]*todos.Todo, error) {
	cursor, err := s.coll.Find(context.Background(), bson.D{})
	if err != nil {
		return nil, err
	}
	var all []*todos.Todo = []*todos.Todo{}
	err = cursor.All(context.Background(), &all)
	if err != nil {
		return nil, err
	}
	return all, nil
}

func NewMongoStorage(connStr string, maxItems int) *MongoStorage {
	ctx := context.Background()
	client, err := mongo.Connect(options.Client().ApplyURI(connStr))
	if err != nil {
		panic(err)
	}

	// Ping the database to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		panic(err)
	}

	db := client.Database("mgm_lab")
	coll := db.Collection("todos")

	return &MongoStorage{
		coll:     coll,
		maxItems: maxItems,
	}
}
