package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	s "github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/storage"
	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"
	"github.com/gin-gonic/gin"
)

type Server struct {
	Port                   int
	Storage                s.TodosStorage
	CleanupIntervalSeconds int
}

// RegisterRoutes wires the CRUD handlers onto a Gin engine. Extracted from
// Start() so handlers can be exercised via httptest.NewServer in unit and
// integration tests without binding a real port.
func RegisterRoutes(engine *gin.Engine, storage s.TodosStorage) {
	group := engine.Group("/api/v1")
	todosGroup := group.Group("/todos")

	todosGroup.GET("", func(c *gin.Context) {
		all, err := storage.ListAll()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, all)
	})

	todosGroup.POST("", func(c *gin.Context) {
		var t todos.Todo
		if err := c.ShouldBindJSON(&t); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		now := time.Now()
		t.CreatedAt = now
		t.UpdatedAt = now
		if err := storage.Create(&t); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, t)
	})

	todosGroup.PUT("", func(c *gin.Context) {
		var t todos.Todo
		if err := c.ShouldBindJSON(&t); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		t.UpdatedAt = time.Now()
		if err := storage.Update(&t); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, t)
	})

	todosGroup.DELETE("", func(c *gin.Context) {
		var t todos.Todo
		if err := c.ShouldBindJSON(&t); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		t.UpdatedAt = time.Now()
		t.Deleted = "true"
		if err := storage.Delete(&t); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, t)
	})
}

func (server *Server) Start() {
	go generateLoad(context.TODO(), server)
	go cleanup(context.TODO(), server)

	engine := gin.Default()
	RegisterRoutes(engine, server.Storage)

	if err := engine.Run("0.0.0.0:" + strconv.Itoa(server.Port)); err != nil {
		panic(err)
	}
}

// cleanupLoop deletes every stored todo on each tick from the supplied
// channel. Pure: caller controls the timing source, so tests drive it
// deterministically with a `make(chan time.Time)` and assert observed
// deletions without sleeping.
func cleanupLoop(ctx context.Context, storage s.TodosStorage, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			all, err := storage.ListAll()
			if err != nil {
				fmt.Println(err)
				continue
			}
			for _, item := range all {
				if err := storage.Delete(item); err != nil {
					fmt.Println(err)
				}
			}
		}
	}
}

// generateLoadLoop publishes a synthetic todo on every tick. Same shape as
// cleanupLoop — pure ticker contract, testable without time.Sleep.
func generateLoadLoop(ctx context.Context, storage s.TodosStorage, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			now := time.Now()
			t := &todos.Todo{Text: "foo", CreatedAt: now, UpdatedAt: now}
			if err := storage.Create(t); err != nil {
				fmt.Println(err)
			}
		}
	}
}

func cleanup(ctx context.Context, server *Server) {
	ticker := time.NewTicker(time.Duration(server.CleanupIntervalSeconds) * time.Second)
	defer ticker.Stop()
	cleanupLoop(ctx, server.Storage, ticker.C)
}

func generateLoad(ctx context.Context, server *Server) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	generateLoadLoop(ctx, server.Storage, ticker.C)
}
