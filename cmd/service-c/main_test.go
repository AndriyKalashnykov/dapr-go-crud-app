package main

import (
	"context"
	"testing"

	"github.com/dapr/go-sdk/service/common"
)

func TestFormatEvent(t *testing.T) {
	tests := []struct {
		name string
		e    *common.TopicEvent
		want string
	}{
		{
			name: "json event",
			e:    &common.TopicEvent{DataContentType: "application/json", ID: "abc-123"},
			want: "event consumed application/json abc-123",
		},
		{
			name: "empty fields",
			e:    &common.TopicEvent{},
			want: "event consumed  ",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatEvent(tc.e); got != tc.want {
				t.Errorf("formatEvent() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHandleEvent(t *testing.T) {
	tests := []struct {
		name string
		e    *common.TopicEvent
	}{
		{name: "typical event", e: &common.TopicEvent{DataContentType: "text/plain", ID: "1"}},
		{name: "empty event", e: &common.TopicEvent{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			retry, err := handleEvent(context.Background(), tc.e)
			if retry {
				t.Errorf("handleEvent() retry = true, want false")
			}
			if err != nil {
				t.Errorf("handleEvent() err = %v, want nil", err)
			}
		})
	}
}
