package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

func zfsHandler(w http.ResponseWriter, r *http.Request) {
	pool, err := zfs.Status(r.Context())
	if err != nil {
		slog.Error("zfsHandler", "error ", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "zfs check failed - " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, pool)
}

func systemHandler(w http.ResponseWriter, _ *http.Request) {
	s, err := system.Collect()
	if err != nil {
		slog.Error("systemHandler", "error ", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "system check failed - " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s)
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
			default:
				eStatus = http.StatusBadRequest
			}
			slog.Error("backupsHandler", "error", err)
			writeJSON(w, eStatus, map[string]string{"error": "backups check failed - " + err.Error()})
			return
		}

		orWidget := Widget{Name: "oldtown restic", Status: c.Status, LastS: formatTime(c.LastPing)}
		writeJSON(w, http.StatusOK, orWidget)
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := godotenv.Load()
	if err != nil {
		slog.Error("error loading .env file")
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.HandleFunc("GET /api/v1/zfs", zfsHandler)
	mux.HandleFunc("GET /api/v1/system", systemHandler)
	hcUrl := os.Getenv("HC_URL")
	hcKey := os.Getenv("HC_KEY")
	if hcUrl == "" || hcKey == "" {
		slog.Error("value(s) not set -", "hcUrl", hcUrl, "hcKey", hcKey)
		os.Exit(1)
	}
	mux.HandleFunc("GET /api/v1/backups", backupsHandler(healthchecks.New(hcUrl, hcKey)))

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
