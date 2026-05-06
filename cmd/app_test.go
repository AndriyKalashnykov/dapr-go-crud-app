package main

import (
	"testing"

	"github.com/AndriyKalashnykov/dapr-go-crud-app/pkg/storage"
)

// selectStorage's `default:` branch dials Mongo (panics on connect failure)
// — covered separately in pkg/storage/mongo_integration_test.go. This file
// pins the type-routing behaviour for the three branches that don't open a
// connection at construction time.
func TestSelectStorage_Routing(t *testing.T) {
	tests := []struct {
		name    string
		connStr string
		wantTyp any
	}{
		{"empty defaults to in-memory", "", &storage.InMemoryStorage{}},
		{"explicit mem", "mem", &storage.InMemoryStorage{}},
		{"dapr returns DaprStorage", "dapr", &storage.DaprStorage{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := selectStorage(tc.connStr, 5)
			switch tc.wantTyp.(type) {
			case *storage.InMemoryStorage:
				if _, ok := got.(*storage.InMemoryStorage); !ok {
					t.Errorf("selectStorage(%q) = %T, want *storage.InMemoryStorage", tc.connStr, got)
				}
			case *storage.DaprStorage:
				if _, ok := got.(*storage.DaprStorage); !ok {
					t.Errorf("selectStorage(%q) = %T, want *storage.DaprStorage", tc.connStr, got)
				}
			}
		})
	}
}
