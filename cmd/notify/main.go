package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/awaleha/stygai-status/internal/notifier"
)

func main() {
	topic := flag.String("topic", "", "ntfy topic")
	tagsFlag := flag.String("tags", "", "comma-separated emoji tags")
	badUsage := "try 'notify --help' for more information "
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Command line tool for publishing to ntfy topics.\n")
		fmt.Fprintf(os.Stderr, "NTFY_URL env variable must be set, --topic flag and message are mandatory\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  notify [options] message\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n notify --topic stygai-alerts \"obsidian backup failed\"\n")
		fmt.Fprintf(os.Stderr, "notify --topic stygai-backups --tags check \"restic: success\"\n")
		fmt.Fprintf(os.Stderr, "notify --topic stygai-alerts --tags \"check,skull\" \"restic: fatal: lock held\"\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	message := strings.Join(flag.Args(), " ")
	if *topic == "" {
		slog.Error("missing topic, " + badUsage)
		os.Exit(2)
	} else if message == "" {
		slog.Error("missing message, " + badUsage)
		os.Exit(2)
	}

	var tags []string
	for _, tag := range strings.Split(*tagsFlag, ",") {
		trimmed := strings.TrimSpace(tag)
		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}

	ntfyURL := os.Getenv("NTFY_URL")
	ntfyToken := os.Getenv("NTFY_TOKEN")
	if ntfyURL == "" {
		slog.Error("NTFY_URL not set")
		os.Exit(2)
	}
	client := notifier.New(ntfyURL, ntfyToken)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	id, err := client.Publish(ctx, *topic, message, tags...)

	var eStatus int
	if err != nil {
		switch {
		case errors.Is(err, notifier.ErrBadGateway):
			eStatus = http.StatusBadGateway
		case errors.Is(err, notifier.ErrUnavailable):
			eStatus = http.StatusServiceUnavailable
		default:
			eStatus = http.StatusInternalServerError
		}
		slog.Error("publish failed -", "status", eStatus, "error", err)
		os.Exit(1)
	}

	slog.Info("published", "topic", *topic, "id", id)
	os.Exit(0)

}
