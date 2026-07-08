package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	port := flag.Int("port", 8080, "port for the server to listen to")
	flag.Parse()

	fmt.Printf("starting dummy app on :%d (accepts requests at the /events path)\n", *port)

	engine := gin.Default()

	group := engine.Group("/events")
	group.GET("", eventsHandler)
	group.POST("", eventsHandler)

	if err := engine.Run(listenAddr(*port)); err != nil {
		panic(err)
	}
}

// envOr returns the value of the environment variable named key, or def when
// the variable is not set.
func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// listenAddr builds the host:port bind address. The host is operator-tunable
// via DUMMY_HOST (default 0.0.0.0) rather than hardcoded.
func listenAddr(port int) string {
	return fmt.Sprintf("%s:%d", envOr("DUMMY_HOST", "0.0.0.0"), port)
}

// eventsHandler responds to both GET and POST on /events with a fixed
// success body. Extracted as a named handler so it can be exercised via
// httptest without binding a real port.
func eventsHandler(c *gin.Context) {
	c.JSON(200, map[string]any{
		"result": true,
	})
}
