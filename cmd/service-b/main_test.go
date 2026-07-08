package main

import (
	"bytes"
	"testing"

	"github.com/dapr/go-sdk/service/common"
)

func TestShouldSleep(t *testing.T) {
	tests := []struct {
		name string
		r    int
		want bool
	}{
		{name: "zero fast path", r: 0, want: false},
		{name: "just below threshold", r: slowPathThreshold - 1, want: false},
		{name: "at threshold sleeps", r: slowPathThreshold, want: true},
		{name: "above threshold sleeps", r: 99, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSleep(tc.r); got != tc.want {
				t.Errorf("shouldSleep(%d) = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}

func TestBuildEchoContent(t *testing.T) {
	tests := []struct {
		name string
		in   *common.InvocationEvent
	}{
		{
			name: "populated invocation echoes all fields",
			in: &common.InvocationEvent{
				Data:        []byte(`{"orderId":"1"}`),
				ContentType: "application/json",
				DataTypeURL: "type.googleapis.com/Order",
			},
		},
		{
			name: "empty invocation",
			in:   &common.InvocationEvent{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildEchoContent(tc.in)
			if !bytes.Equal(got.Data, tc.in.Data) {
				t.Errorf("Data = %q, want %q", got.Data, tc.in.Data)
			}
			if got.ContentType != tc.in.ContentType {
				t.Errorf("ContentType = %q, want %q", got.ContentType, tc.in.ContentType)
			}
			if got.DataTypeURL != tc.in.DataTypeURL {
				t.Errorf("DataTypeURL = %q, want %q", got.DataTypeURL, tc.in.DataTypeURL)
			}
		})
	}
}
