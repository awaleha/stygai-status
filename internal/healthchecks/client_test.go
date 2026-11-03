package healthchecks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientCheck(t *testing.T) {
	t.Run("last_ping set", func(t *testing.T) {
		var gotKey string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotKey = r.Header.Get("X-Api-Key")
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			if r.URL.Path != "/api/v1/checks/" {
				t.Errorf("path = %q, want /api/v1/checks/", r.URL.Path)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"checks": []map[string]any{
					{"name": "oldtown-restic", "status": "up", "last_ping": "2026-11-02T15:37:48Z"},
				},
			})
		}))
		defer ts.Close()

		c, err := New(ts.URL, "test-key").Check(context.Background(), "oldtown-restic")
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		want := time.Date(2026, 11, 2, 15, 37, 48, 0, time.UTC)
		if c.Name != "oldtown-restic" || c.Status != "up" || c.LastPing == nil || !c.LastPing.Equal(want) {
			t.Errorf("check = %+v, want name oldtown-restic, status up, last_ping %v", c, want)
		}
		if gotKey != "test-key" {
			t.Errorf("X-Api-Key = %q, want test-key", gotKey)
		}
	})

	t.Run("last_ping null", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{
				"checks": []map[string]any{
					{"name": "oldtown-restic", "status": "down", "last_ping": nil},
				},
			})
		}))
		defer ts.Close()

		c, err := New(ts.URL, "test-key").Check(context.Background(), "oldtown-restic")
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if c.LastPing != nil {
			t.Errorf("LastPing = %v, want nil", *c.LastPing)
		}
		if c.Status != "down" {
			t.Errorf("status = %q, want down", c.Status)
		}
	})

	t.Run("check absent", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"checks": []map[string]any{}})
		}))
		defer ts.Close()

		_, err := New(ts.URL, "test-key").Check(context.Background(), "oldtown-restic")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("non-2xx", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer ts.Close()

		_, err := New(ts.URL, "test-key").Check(context.Background(), "oldtown-restic")
		if !errors.Is(err, ErrBadGateway) {
			t.Errorf("err = %v, want ErrBadGateway", err)
		}
	})
}
