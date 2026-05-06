package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/timeline"
	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/todos"
	"github.com/gin-gonic/gin"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

// RegisterRoutes wires the timeline handlers onto a Gin engine. Extracted
// from main() so the CloudEvent-vs-raw decoding branch can be table-tested
// via httptest without binding a port.
func RegisterRoutes(engine *gin.Engine, tl timeline.Timeline) {
	eventsGroup := engine.Group("/").Group("todos")

	eventsGroup.GET("", func(c *gin.Context) {
		c.JSON(http.StatusOK, tl.Timeline())
	})

	eventsGroup.POST("", func(c *gin.Context) {
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
		}

		todo, err := decodeTodo(bodyBytes)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		tl.Handle(todo)
		c.Writer.WriteHeader(http.StatusOK)
	})
}

// decodeTodo returns the embedded todo from either a structured CloudEvent
// envelope OR a raw JSON Todo body — Dapr's `rawPayload` metadata flips the
// pubsub publisher between the two shapes, so the subscriber must handle
// both. Pure function, easy to table-test.
func decodeTodo(body []byte) (todos.Todo, error) {
	var todo todos.Todo

	event := cloudevents.NewEvent()
	if err := json.Unmarshal(body, &event); err == nil && event.ID() != "" {
		// Structured CloudEvent — the Todo lives inside event.Data().
		if dataErr := json.Unmarshal(event.Data(), &todo); dataErr == nil {
			return todo, nil
		}
	}

	// Raw payload — body IS the Todo JSON.
	if err := json.Unmarshal(body, &todo); err != nil {
		return todo, err
	}
	return todo, nil
}

func main() {
	serveFlagSet := flag.NewFlagSet("timeline app serve", flag.ExitOnError)
	serverPort := serveFlagSet.Int("port", 8080, "port for the server to listen to")

	tl := timeline.New()

	engine := gin.Default()
	RegisterRoutes(engine, tl)

	fmt.Printf("Starting timeline server on port %d\n", *serverPort)
	if err := engine.Run("0.0.0.0:" + strconv.Itoa(*serverPort)); err != nil {
		panic(err)
	}
}
