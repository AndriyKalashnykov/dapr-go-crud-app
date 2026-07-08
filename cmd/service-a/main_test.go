package main

import (
	"os"
	"testing"
)

func TestBuildInvokeURL(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		port   string
		app    string
		method string
		want   string
	}{
		{
			name:   "default host and port",
			host:   "http://localhost",
			port:   "3500",
			app:    "service-b",
			method: "hello",
			want:   "http://localhost:3500/v1.0/invoke/service-b/method/hello",
		},
		{
			name:   "custom host and port",
			host:   "http://dapr.internal",
			port:   "3501",
			app:    "orders",
			method: "create",
			want:   "http://dapr.internal:3501/v1.0/invoke/orders/method/create",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildInvokeURL(tc.host, tc.port, tc.app, tc.method); got != tc.want {
				t.Errorf("buildInvokeURL(%q,%q,%q,%q) = %q, want %q",
					tc.host, tc.port, tc.app, tc.method, got, tc.want)
			}
		})
	}
}

func TestEnvOr(t *testing.T) {
	const key = "DAPR_GO_CRUD_TEST_ENVOR" // unlikely to collide with real env

	tests := []struct {
		name  string
		set   bool
		value string
		def   string
		want  string
	}{
		{name: "unset returns default", set: false, def: "3500", want: "3500"},
		{name: "set returns value", set: true, value: "3501", def: "3500", want: "3501"},
		{name: "set empty returns empty", set: true, value: "", def: "3500", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv registers restoration of the original (unset) state at
			// cleanup, so mutating the slot below is safe and isolated.
			t.Setenv(key, "")
			if tc.set {
				t.Setenv(key, tc.value)
			} else {
				_ = os.Unsetenv(key)
			}
			if got := envOr(key, tc.def); got != tc.want {
				t.Errorf("envOr(%q, %q) = %q, want %q", key, tc.def, got, tc.want)
			}
		})
	}
}
