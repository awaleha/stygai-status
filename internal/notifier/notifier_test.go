package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublish(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var got struct {
			Topic   string   `json:"topic"`
			Message string   `json:"message"`
			Tags    []string `json:"tags"`
		}
		var gotAuth, gotCT string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotCT = r.Header.Get("Content-Type")
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("decode body: %v", err)
			}
			json.NewEncoder(w).Encode(map[string]string{"id": "abc123"})
		}))
		defer ts.Close()

		id, err := New(ts.URL, "test-token").Publish(context.Background(), "backups", "restic finished", "backup", "stygai")
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if id != "abc123" {
			t.Errorf("id = %q, want abc123", id)
		}
		if got.Topic != "backups" || got.Message != "restic finished" {
			t.Errorf("body topic/message = %q/%q, want backups/restic finished", got.Topic, got.Message)
		}
		if len(got.Tags) != 2 || got.Tags[0] != "backup" || got.Tags[1] != "stygai" {
			t.Errorf("tags = %v, want [backup stygai]", got.Tags)
		}
		if gotAuth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", gotAuth)
		}
		if gotCT != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", gotCT)
		}
	})

	t.Run("no token sent", func(t *testing.T) {
		var gotAuth string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			json.NewEncoder(w).Encode(map[string]string{"id": "x"})
		}))
		defer ts.Close()

		if _, err := New(ts.URL, "").Publish(context.Background(), "t", "m"); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if gotAuth != "" {
			t.Errorf("Authorization = %q, want empty", gotAuth)
		}
	})

	t.Run("non-2xx", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer ts.Close()

		_, err := New(ts.URL, "").Publish(context.Background(), "t", "m")
		if !errors.Is(err, ErrBadGateway) {
			t.Errorf("err = %v, want ErrBadGateway", err)
		}
	})
}
