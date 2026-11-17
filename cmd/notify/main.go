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

var version = "dev"
var commit = "unknown"

func main() {
	topic := flag.String("topic", "", "ntfy topic")
	tagsFlag := flag.String("tags", "", "comma-separated emoji tags")
	hasV := flag.Bool("version", false, "version of this tool")
	badUsage := "try 'notify --help' for more information "
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Command line tool for publishing to ntfy topics.\n")
		fmt.Fprintf(os.Stderr, "NTFY_URL env variable must be set, --topic flag and message are mandatory\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  notify [options] message\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n notify --topic alerts \"obsidian backup failed\"\n")
		fmt.Fprintf(os.Stderr, "notify --topic backups --tags check \"restic: success\"\n")
		fmt.Fprintf(os.Stderr, "notify --topic alerts --tags \"check,skull\" \"restic: fatal: lock held\"\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *hasV {
		fmt.Printf("notify %s (commit %s)\n", version, commit)
		os.Exit(0)
	}

	message := strings.Join(flag.Args(), " ")
	if *topic == "" {
		slog.Error("missing topic, " + badUsage)
		os.Exit(2)
	} else if message == "" {
		slog.Error("missing message, " + badUsage)
		os.Exit(2)
	}

	tags := splitTags(*tagsFlag)

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

	if err != nil {
		eStatus := publishStatus(err)
		slog.Error("publish failed -", "status", eStatus, "error", err)
		os.Exit(1)
	}

	slog.Info("published", "topic", *topic, "id", id)
	os.Exit(0)

}

// splitTags turns the --tags flag into the tag list: comma-split,
// whitespace-trimmed, empty entries dropped.
func splitTags(tagsFlag string) []string {
	var tags []string
	for _, tag := range strings.Split(tagsFlag, ",") {
		trimmed := strings.TrimSpace(tag)
		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}

// publishStatus maps a Publish error to the HTTP-style status the CLI
// reports before exiting; anything unmapped falls through to 500.
func publishStatus(err error) int {
	switch {
	case errors.Is(err, notifier.ErrBadGateway):
		return http.StatusBadGateway
	case errors.Is(err, notifier.ErrUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
