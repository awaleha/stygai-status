package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/awaleha/stygai-status/internal/docker"
	"github.com/awaleha/stygai-status/internal/healthchecks"
	"github.com/awaleha/stygai-status/internal/system"
	"github.com/awaleha/stygai-status/internal/zfs"
)

func TestZFSHandler(t *testing.T) {
	// LastScrub stays nil so the round-tripped Pool compares with != below.
	pool := zfs.Pool{Name: "stag", Health: "ONLINE", SizeBytes: 19991291842560, FreeBytes: 11983096561664, CapPct: 40}

	t.Run("200 fake pool", func(t *testing.T) {
		handler := zfsHandler(func(ctx context.Context) (zfs.Pool, error) {
			return pool, nil
		})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/zfs", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var got zfs.Pool
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got != pool {
			t.Errorf("pool = %+v, want %+v", got, pool)
		}
	})

	t.Run("500 fake error", func(t *testing.T) {
		handler := zfsHandler(func(ctx context.Context) (zfs.Pool, error) {
			return zfs.Pool{}, errors.New("boom")
		})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/zfs", nil))

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
		var body map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		want := "zfs check failed - boom"
		if body["error"] != want {
			t.Errorf("error = %q, want %q", body["error"], want)
		}
	})

	t.Run("405 through the real mux", func(t *testing.T) {
		mux := newTestMux()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/zfs", nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
		if allow := rec.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
			t.Errorf("Allow = %q, want it to include %q", allow, http.MethodGet)
		}
	})
}

// newTestMux returns the production route table with a dummy Healthchecks
// client — no handler in these tests ever reaches the network.
func newTestMux() *http.ServeMux {
	return newMux("http://localhost:8000/hc", "test-key")
}

func TestSystemHandler(t *testing.T) {
	stats := system.Stats{
		Hostname: "stygai",
		CPUPct:   12.5,
		CPUs:     map[string]float64{"cpu0": 5.5, "cpu1": 7.5},
		MemUsed:  8589934592,
		MemTotal: 17179869184,
		Uptime:   864000,
		Main:     system.Disk{Path: "/", Used: 100, Total: 500, UsedPercent: 20},
		Stag:     system.Disk{Path: "/stag", Used: 200, Total: 1000, UsedPercent: 20},
		StagFree: 800,
		LoadAvg:  system.Loads{Load1: 0.5, Load5: 0.6, Load15: 0.7},
	}

	t.Run("200 fake stats", func(t *testing.T) {
		handler := systemHandler(func() (system.Stats, error) {
			return stats, nil
		})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/system", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var got system.Stats
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if !reflect.DeepEqual(got, stats) {
			t.Errorf("stats = %+v, want %+v", got, stats)
		}
	})

	t.Run("500 fake error", func(t *testing.T) {
		handler := systemHandler(func() (system.Stats, error) {
			return system.Stats{}, errors.New("boom")
		})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/system", nil))

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
		var body map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if want := "system check failed - boom"; body["error"] != want {
			t.Errorf("error = %q, want %q", body["error"], want)
		}
	})

	t.Run("405 through the real mux", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newTestMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/system", nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
		if allow := rec.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
			t.Errorf("Allow = %q, want it to include %q", allow, http.MethodGet)
		}
	})
}

type fakeChecker struct {
	check healthchecks.Check
	err   error
}

func (f fakeChecker) Check(_ context.Context, _ string) (healthchecks.Check, error) {
	return f.check, f.err
}

func TestBackupsHandler(t *testing.T) {
	lastPing := time.Now().Add(-90 * time.Minute)
	check := healthchecks.Check{Name: "oldtown-restic", Status: "up", LastPing: &lastPing}

	t.Run("200 fake check", func(t *testing.T) {
		handler := backupsHandler(fakeChecker{check: check})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var got Widget
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		want := Widget{Name: "oldtown restic", Status: "up", LastS: "1h 30m ago"}
		if got != want {
			t.Errorf("widget = %+v, want %+v", got, want)
		}
	})

	t.Run("404 unknown check", func(t *testing.T) {
		handler := backupsHandler(fakeChecker{err: healthchecks.ErrNotFound})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("500 fake error", func(t *testing.T) {
		handler := backupsHandler(fakeChecker{err: errors.New("boom")})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil))

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
		var body map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if want := "backups check failed - boom"; body["error"] != want {
			t.Errorf("error = %q, want %q", body["error"], want)
		}
	})

	t.Run("405 through the real mux", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newTestMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/backups", nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
		if allow := rec.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
			t.Errorf("Allow = %q, want it to include %q", allow, http.MethodGet)
		}
	})
}

func TestDockerHandler(t *testing.T) {
	containers := []docker.Container{
		{Name: "homepage", State: "running", Status: "Up 3 days", Health: "healthy"},
		{Name: "healthchecks", State: "running", Status: "Up 3 days", Health: ""},
	}

	t.Run("200 fake list", func(t *testing.T) {
		handler := dockerHandler(func(ctx context.Context) ([]docker.Container, error) {
			return containers, nil
		})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/docker", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var got []docker.Container
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if !reflect.DeepEqual(got, containers) {
			t.Errorf("containers = %+v, want %+v", got, containers)
		}
	})

	t.Run("503 unavailable", func(t *testing.T) {
		handler := dockerHandler(func(ctx context.Context) ([]docker.Container, error) {
			return nil, docker.ErrUnavailable
		})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/docker", nil))

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("500 fake error", func(t *testing.T) {
		handler := dockerHandler(func(ctx context.Context) ([]docker.Container, error) {
			return nil, errors.New("boom")
		})
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/docker", nil))

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
		var body map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if want := "docker list failed - boom"; body["error"] != want {
			t.Errorf("error = %q, want %q", body["error"], want)
		}
	})

	t.Run("405 through the real mux", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newTestMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/docker", nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
		if allow := rec.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
			t.Errorf("Allow = %q, want it to include %q", allow, http.MethodGet)
		}
	})
}
