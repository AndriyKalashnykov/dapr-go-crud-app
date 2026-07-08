package main

import (
	"context"
	"fmt"

	"github.com/dapr/go-sdk/service/common"
	"github.com/dapr/go-sdk/service/grpc"
)

var pubsubName string = "pubsub"
var pubsubTopic string = "events"

func main() {
	fmt.Println("starting service-c app (consumes from events topic)")

	s, err := grpc.NewService(":8080")
	if err != nil {
		panic(err)
	}
	if err = s.AddTopicEventHandler(&common.Subscription{
		PubsubName: pubsubName,
		Topic:      pubsubTopic,
	}, handleEvent); err != nil {
		panic(err)
	}

	err = s.Start()
	if err != nil {
		panic(err)
	}
}

// formatEvent renders the log line for a consumed topic event. Pure so the
// message shape can be asserted in a unit test.
func formatEvent(e *common.TopicEvent) string {
	return fmt.Sprintf("event consumed %s %s", e.DataContentType, e.ID)
}

// handleEvent is the topic-event subscriber handler. It always acknowledges
// (retry=false, err=nil), matching the demo's fire-and-forget semantics.
func handleEvent(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	fmt.Println(formatEvent(e))
	return false, nil
}
