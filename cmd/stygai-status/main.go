package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/awaleha/stygai-status/internal/docker"
	"github.com/awaleha/stygai-status/internal/healthchecks"
	"github.com/awaleha/stygai-status/internal/system"
	"github.com/awaleha/stygai-status/internal/zfs"
	"github.com/joho/godotenv"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"dur", time.Since(start))
	})
}

func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("ok"))
}

type zfsReader func(context.Context) (zfs.Pool, error)

func zfsHandler(read zfsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pool, err := read(r.Context())
		if err != nil {
			slog.Error("zfsHandler", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "zfs check failed - " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, pool)
	}
}

type systemReader func() (system.Stats, error)

func systemHandler(read systemReader) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		s, err := read()
		if err != nil {
			slog.Error("systemHandler", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "system check failed - " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, s)
	}
}

type checker interface {
	Check(ctx context.Context, name string) (healthchecks.Check, error)
}

type Widget struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	LastS  string `json:"last_success"`
}

func formatTime(lastPing *time.Time) string {
	if lastPing == nil {
		return "never"
	}
	d := time.Since(*lastPing)
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm ago", hours, minutes)
}

type listener func(ctx context.Context) ([]docker.Container, error)

func backupsHandler(hc checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := hc.Check(r.Context(), "oldtown-restic")
		var eStatus int
		if err != nil {
			switch {
			case errors.Is(err, healthchecks.ErrNotFound):
				eStatus = http.StatusNotFound
			case errors.Is(err, healthchecks.ErrBadRequest):
				eStatus = http.StatusBadRequest
			case errors.Is(err, healthchecks.ErrBadGateway):
				eStatus = http.StatusBadGateway
			case errors.Is(err, healthchecks.ErrUnavailable):
				eStatus = http.StatusServiceUnavailable
			default:
				eStatus = http.StatusInternalServerError
			}
			slog.Error("backupsHandler", "error", err)
			writeJSON(w, eStatus, map[string]string{"error": "backups check failed - " + err.Error()})
			return
		}

		orWidget := Widget{Name: "oldtown restic", Status: c.Status, LastS: formatTime(c.LastPing)}
		writeJSON(w, http.StatusOK, orWidget)
	}
}

func dockerHandler(list listener) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, err := list(r.Context())
		var eStatus int
		if err != nil {
			switch {
			case errors.Is(err, docker.ErrBadRequest):
				eStatus = http.StatusBadRequest
			case errors.Is(err, docker.ErrBadGateway):
				eStatus = http.StatusBadGateway
			case errors.Is(err, docker.ErrUnavailable):
				eStatus = http.StatusServiceUnavailable
			default:
				eStatus = http.StatusInternalServerError
			}
			slog.Error("dockerHandler", "error", err)
			writeJSON(w, eStatus, map[string]string{"error": "docker list failed - " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, cs)
	}
}

func newMux(hcUrl, hcKey string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.HandleFunc("GET /api/v1/zfs", zfsHandler(zfs.Status))
	mux.HandleFunc("GET /api/v1/system", systemHandler(system.Collect))
	mux.HandleFunc("GET /api/v1/backups", backupsHandler(healthchecks.New(hcUrl, hcKey)))
	mux.HandleFunc("GET /api/v1/docker", dockerHandler(docker.List))
	return mux
}

var version = "dev"
var commit = "unknown"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hasV := flag.Bool("version", false, "version of this tool")
	flag.Parse()
	if *hasV {
		fmt.Printf("stygai-status %s (commit %s)\n", version, commit)
		os.Exit(0)
	}

	if flag.Arg(0) == "healthcheck" {
		client := &http.Client{
			Timeout: 2 * time.Second,
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:8082/healthz", nil)
		if err != nil {
			slog.Error("creating healthcheck request")
			os.Exit(1)
		}
		resp, err := client.Do(req)
		if err != nil {
			slog.Error("executing healthcheck request")
			os.Exit(1)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("reading healthcheck response body")
			os.Exit(1)
		}
		if resp.StatusCode == 200 && string(body) == "ok" {
			os.Exit(0)
		} else {
			slog.Error("healthcheck response:", "status", resp.Status, "body", string(body))
			os.Exit(1)
		}

	}

	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		slog.Error("error loading .env file", "err", err)
		os.Exit(1)
	}
	hcUrl := os.Getenv("HC_URL")
	hcKey := os.Getenv("HC_KEY")
	if hcUrl == "" || hcKey == "" {
		slog.Error("value(s) not set -", "hcUrl", hcUrl, "hcKey", hcKey)
		os.Exit(1)
	}
	mux := newMux(hcUrl, hcKey)

	srv := &http.Server{
		Addr:              ":8082",
		Handler:           withRequestLog(mux),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown", "err", err)
		}
	}()

	slog.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
	<-done
}
