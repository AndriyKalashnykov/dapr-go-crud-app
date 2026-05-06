package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/timeline"
	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"
	"github.com/gin-gonic/gin"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

func TestDecodeTodo_RawPayload(t *testing.T) {
	want := todos.Todo{Text: "buy milk", Done: "false"}
	body, _ := json.Marshal(want)

	got, err := decodeTodo(body)
	if err != nil {
		t.Fatalf("decodeTodo: %v", err)
	}
	if got.Text != want.Text || got.Done != want.Done {
		t.Errorf("decoded todo = %+v, want %+v", got, want)
	}
}

func TestDecodeTodo_StructuredCloudEvent(t *testing.T) {
	td := todos.Todo{Text: "ship feature", Done: "true"}

	ev := cloudevents.NewEvent()
	ev.SetID("evt-1")
	ev.SetSource("crud-app")
	ev.SetType("com.example.todos")
	ev.SetTime(time.Now())
	if err := ev.SetData(cloudevents.ApplicationJSON, td); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	body, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal CloudEvent: %v", err)
	}

	got, err := decodeTodo(body)
	if err != nil {
		t.Fatalf("decodeTodo: %v", err)
	}
	if got.Text != td.Text || got.Done != td.Done {
		t.Errorf("decoded todo = %+v, want %+v", got, td)
	}
}

func TestDecodeTodo_GarbageReturnsError(t *testing.T) {
	if _, err := decodeTodo([]byte("not-json")); err == nil {
		t.Fatal("decodeTodo accepted invalid JSON, want error")
	}
}

func TestRegisterRoutes_PostThenGetReturnsHandledTodo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tl := timeline.New()
	engine := gin.New()
	RegisterRoutes(engine, tl)

	srv := httptest.NewServer(engine)
	defer srv.Close()

	body, _ := json.Marshal(todos.Todo{Text: "feed cat"})
	resp, err := http.Post(srv.URL+"/todos", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, want 200", resp.StatusCode)
	}

	getResp, err := http.Get(srv.URL + "/todos")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = getResp.Body.Close() }()

	var entries []string
	if err := json.NewDecoder(getResp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode GET body: %v", err)
	}
	if len(entries) != 1 || entries[0] != "New todo created: feed cat" {
		t.Errorf("Timeline entries = %v, want one 'New todo created: feed cat'", entries)
	}
}

func TestRegisterRoutes_PostBadJsonReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tl := timeline.New()
	engine := gin.New()
	RegisterRoutes(engine, tl)

	srv := httptest.NewServer(engine)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/todos", "application/json", bytes.NewReader([]byte("not-json")))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST status = %d, want 400", resp.StatusCode)
	}
}
