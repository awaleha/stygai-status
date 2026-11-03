package main

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/awaleha/stygai-status/internal/notifier"
)

func TestSplitTags(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"two tags", "check,skull", []string{"check", "skull"}},
		{"trims and drops empties", "check,, skull ", []string{"check", "skull"}},
		{"empty flag gives no tags", "", nil},
		{"single tag trimmed", " single ", []string{"single"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitTags(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitTags(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestPublishStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"bad gateway", notifier.ErrBadGateway, http.StatusBadGateway},
		{"unavailable", notifier.ErrUnavailable, http.StatusServiceUnavailable},
		{"unknown falls through to 500", errors.New("boom"), http.StatusInternalServerError},
		{"nil falls through to 500", nil, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publishStatus(tt.err); got != tt.want {
				t.Errorf("publishStatus(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
