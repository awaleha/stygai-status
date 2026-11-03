package zfs

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func Test_parse(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		listOut string
		scanOut string
		nextOut string
		want    Pool
		wantErr bool
	}{
		{
			name:    "Finished scrub",
			listOut: "stag\t19991291842560\t11983096561664\t40\tONLINE\n",
			scanOut: "  scan: scrub repaired 0B in 00:01:30 with 2 errors on Sun Nov  2 15:37:48 2025\n",
			nextOut: "Sun 2026-11-09 00:00:00 CET\n",
			want: Pool{
				Name:        "stag",
				Health:      "ONLINE",
				SizeBytes:   19991291842560,
				FreeBytes:   11983096561664,
				CapPct:      40,
				LastScrub:   timePtr(time.Date(2025, 11, 2, 15, 37, 48, 0, time.Local)),
				ScrubErrors: 2,
				NextScrub:   time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name:    "Scrub in progress",
			listOut: "stag\t19991291842560\t11983096561664\t40\tONLINE\n",
			scanOut: "  scan: scrub in progress since Sun Nov  2 15:37:48 2025\n",
			nextOut: "Sun 2026-11-09 00:00:00 CET\n",
			want: Pool{
				Name:      "stag",
				Health:    "ONLINE",
				SizeBytes: 19991291842560,
				FreeBytes: 11983096561664,
				CapPct:    40,
				LastScrub: timePtr(time.Date(2025, 11, 2, 15, 37, 48, 0, time.Local)),
				NextScrub: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name:    "Scrub canceled",
			listOut: "stag\t19991291842560\t11983096561664\t40\tONLINE\n",
			scanOut: "  scan: scrub canceled on Sun Nov  2 15:37:48 2025\n",
			nextOut: "Sun 2026-11-09 00:00:00 CET\n",
			want: Pool{
				Name:      "stag",
				Health:    "ONLINE",
				SizeBytes: 19991291842560,
				FreeBytes: 11983096561664,
				CapPct:    40,
				LastScrub: timePtr(time.Date(2025, 11, 2, 15, 37, 48, 0, time.Local)),
				NextScrub: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			// The zero-time decision: with no scan line, LastScrub stays
			// nil and omitempty drops it from the JSON (pinned in TestNoScanJSON).
			name:    "No scan line",
			listOut: "stag\t19991291842560\t11983096561664\t40\tONLINE\n",
			scanOut: "  pool: stag\n state: ONLINE\n",
			nextOut: "Sun 2026-11-09 00:00:00 CET\n",
			want: Pool{
				Name:      "stag",
				Health:    "ONLINE",
				SizeBytes: 19991291842560,
				FreeBytes: 11983096561664,
				CapPct:    40,
				NextScrub: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name:    "Malformed size",
			listOut: "stag\tnot-a-size\t11983096561664\t40\tONLINE\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := parse(tt.listOut, tt.scanOut, tt.nextOut)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("parse() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("parse() succeeded unexpectedly")
			}
			if !poolEqual(got, tt.want) {
				t.Errorf("parse() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// poolEqual compares the scalar fields with == and the two timestamps
// with time.Equal, which ignores location and monotonic-clock details
// that the fixtures don't pin.
func poolEqual(a, b Pool) bool {
	if a.Name != b.Name || a.Health != b.Health || a.SizeBytes != b.SizeBytes ||
		a.FreeBytes != b.FreeBytes || a.CapPct != b.CapPct || a.ScrubErrors != b.ScrubErrors ||
		!a.NextScrub.Equal(b.NextScrub) {
		return false
	}
	if a.LastScrub == nil || b.LastScrub == nil {
		return a.LastScrub == b.LastScrub
	}
	return a.LastScrub.Equal(*b.LastScrub)
}

// TestNoScanJSON pins the A7 zero-time decision: LastScrub is *time.Time
// with omitempty, so a pool with no scan line marshals without the key —
// M6 renders a blank, not 0001-01-01T00:00:00Z.
func TestNoScanJSON(t *testing.T) {
	noScan, err := parse("stag\t19991291842560\t11983096561664\t40\tONLINE\n",
		"  pool: stag\n state: ONLINE\n",
		"Sun 2026-11-09 00:00:00 CET\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b, err := json.Marshal(noScan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "last_scrub") {
		t.Errorf("no-scan pool marshals last_scrub; want it omitted: %s", b)
	}
	if strings.Contains(string(b), "0001-01-01") {
		t.Errorf("no-scan pool marshals the zero time: %s", b)
	}

	scrubbed := Pool{LastScrub: timePtr(time.Date(2025, 11, 2, 15, 37, 48, 0, time.Local))}
	b2, err := json.Marshal(scrubbed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b2), "last_scrub") {
		t.Errorf("scrubbed pool omits last_scrub; want it present: %s", b2)
	}
}

func timePtr(t time.Time) *time.Time { return &t }
