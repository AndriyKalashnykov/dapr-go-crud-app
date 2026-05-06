package main

import (
	"flag"
	"fmt"

	"github.com/gin-gonic/gin"
)

func main() {
	port := flag.Int("port", 8080, "port for the server to listen to")
	flag.Parse()

	fmt.Printf("starting dummy app on :%d (accepts requests at the /events path)\n", *port)

	engine := gin.Default()

	group := engine.Group("/events")

	group.GET("", func(c *gin.Context) {
		c.JSON(200, map[string]any{
			"result": true,
		})
	})

	group.POST("", func(c *gin.Context) {
		c.JSON(200, map[string]any{
			"result": true,
		})
	})

	if err := engine.Run(fmt.Sprintf("0.0.0.0:%d", *port)); err != nil {
		panic(err)
	}
}
