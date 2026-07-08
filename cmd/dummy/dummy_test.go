package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestListenAddr(t *testing.T) {
	tests := []struct {
		name string
		host string // "" means DUMMY_HOST unset (default applies)
		port int
		want string
	}{
		{name: "default host", host: "", port: 8080, want: "0.0.0.0:8080"},
		{name: "custom host and port", host: "127.0.0.1", port: 9090, want: "127.0.0.1:9090"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv registers restoration of the original state at cleanup,
			// so mutating the slot below is safe and isolated.
			t.Setenv("DUMMY_HOST", "")
			if tc.host == "" {
				_ = os.Unsetenv("DUMMY_HOST") // exercise the default path
			} else {
				t.Setenv("DUMMY_HOST", tc.host)
			}
			if got := listenAddr(tc.port); got != tc.want {
				t.Errorf("listenAddr(%d) = %q, want %q", tc.port, got, tc.want)
			}
		})
	}
}

func TestEventsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		method string
	}{
		{name: "GET", method: http.MethodGet},
		{name: "POST", method: http.MethodPost},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			group := engine.Group("/events")
			group.GET("", eventsHandler)
			group.POST("", eventsHandler)

			req := httptest.NewRequest(tc.method, "/events", nil)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got, want := rec.Body.String(), `{"result":true}`; got != want {
				t.Errorf("body = %q, want %q", got, want)
			}
		})
	}
}
