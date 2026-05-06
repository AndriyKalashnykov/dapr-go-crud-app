//go:build integration

package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/server"
	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/storage"
	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"
	"github.com/gin-gonic/gin"
)

// newTestServer wires the real handler set against InMemoryStorage and
// exposes it through net/http/httptest. No real port binding, no infra
// dependency — the integration boundary here is "Gin routing + handler
// + storage interface" working together end-to-end.
func newTestServer(t *testing.T) (*httptest.Server, storage.TodosStorage) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := storage.NewInMemoryStorage(20)
	engine := gin.New()
	server.RegisterRoutes(engine, store)
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv, store
}

func postJSON(t *testing.T, url string, payload any) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestCRUDLifecycle_PostListPutDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	url := srv.URL + "/api/v1/todos"

	// CREATE
	createResp := postJSON(t, url, todos.Todo{Text: "buy milk"})
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, want 200", createResp.StatusCode)
	}
	var created todos.Todo
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode POST body: %v", err)
	}
	createResp.Body.Close()
	if created.Id == "" {
		t.Fatal("POST response missing Id")
	}
	if created.Text != "buy milk" {
		t.Errorf("POST response Text = %q, want 'buy milk'", created.Text)
	}

	// LIST
	listResp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", listResp.StatusCode)
	}
	var listed []todos.Todo
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode GET body: %v", err)
	}
	listResp.Body.Close()
	if len(listed) != 1 || listed[0].Id != created.Id {
		t.Fatalf("GET = %+v, want single item with id %q", listed, created.Id)
	}

	// UPDATE
	created.Done = "true"
	created.Text = "buy oat milk"
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(mustMarshal(t, created)))
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Errorf("PUT status = %d, want 200", putResp.StatusCode)
	}

	// DELETE
	delReq, _ := http.NewRequest(http.MethodDelete, url, bytes.NewReader(mustMarshal(t, created)))
	delReq.Header.Set("Content-Type", "application/json")
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Errorf("DELETE status = %d, want 200", delResp.StatusCode)
	}
}

func TestPost_BadJsonReturns400(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := http.Post(srv.URL+"/api/v1/todos", "application/json", bytes.NewReader([]byte("not-json")))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST bad-JSON status = %d, want 400", resp.StatusCode)
	}
}

func TestPost_MissingRequiredFieldReturns400(t *testing.T) {
	srv, _ := newTestServer(t)

	// Todo.Text has `binding:"required"`, so empty Text must be rejected.
	resp, err := http.Post(srv.URL+"/api/v1/todos", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST missing-Text status = %d, want 400 (Text is required)", resp.StatusCode)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
