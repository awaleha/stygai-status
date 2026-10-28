package zfs

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Pool struct {
	Name        string    `json:"name"`
	Health      string    `json:"health"`
	SizeBytes   uint64    `json:"size_bytes"`
	FreeBytes   uint64    `json:"free_bytes"`
	CapPct      int       `json:"cap_pct"`
	LastScrub   time.Time `json:"last_scrub"`
	ScrubErrors int       `json:"scrub_errors"`
}

func Status(ctx context.Context) (Pool, error) {
	out, err := exec.CommandContext(ctx, "zpool", "list", "-pH", "-o", "name,size,free,capacity,health", "stag").Output()
	if err != nil {
		return Pool{}, fmt.Errorf("zpool list: %w", err)
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))

	size, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return Pool{}, fmt.Errorf("size conversion: %w", err)
	}

	free, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return Pool{}, fmt.Errorf("free conversion: %w", err)
	}

	cap, err := strconv.Atoi(fields[3])
	if err != nil {
		return Pool{}, fmt.Errorf("capacity conversion: %w", err)
	}

	out2, err := exec.CommandContext(ctx, "zpool", "status", "stag").Output()
	if err != nil {
		return Pool{}, fmt.Errorf("zpool status: %w", err)
	}

	var scanLine string
	for _, line := range strings.Split(string(out2), "\n") {
		if strings.HasPrefix(line, "  scan:") {
			scanLine = line
			break
		}
	}

	var dateIndex int
	var serr int
	if strings.HasPrefix(scanLine, "  scan: scrub repaired") {

		dateIndex = strings.Index(scanLine, " on ") + 4
		errIndex1 := strings.Index(scanLine, "with") + 5
		errIndex2 := strings.Index(scanLine, "errors") - 1

		serr, err = strconv.Atoi(scanLine[errIndex1:errIndex2])
		if err != nil {
			return Pool{}, fmt.Errorf("scruberror conversion: %w", err)
		}

	} else if strings.HasPrefix(scanLine, "  scan: scrub in progress") {

		dateIndex = strings.Index(scanLine, "since") + 6

	} else if strings.HasPrefix(scanLine, "  scan: scrub canceled on") {

		dateIndex = strings.Index(scanLine, " on ") + 4

	} else { //no previous scan found
		return Pool{Name: fields[0], Health: fields[4], SizeBytes: size, FreeBytes: free, CapPct: cap, LastScrub: time.Time{}, ScrubErrors: 0}, nil
	}

	layout := "Mon Jan  2 15:04:05 2006"
	t, err := time.Parse(layout, scanLine[dateIndex:])
	if err != nil {
		return Pool{}, fmt.Errorf("time conversion: %w", err)
	}

	pool := Pool{Name: fields[0], Health: fields[4], SizeBytes: size, FreeBytes: free, CapPct: cap, LastScrub: t, ScrubErrors: serr}

	return pool, nil
}
