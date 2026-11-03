package system

import (
	"reflect"
	"testing"
)

func TestRoundTo2d(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"half rounds up", 12.346, 12.35},
		// The nearest float64 to 12.345 sits a hair above it, so it rounds
		// up — pinned so a "rounding fix" is a conscious change, not drift.
		{"float edge rounds up", 12.345, 12.35},
		// The nearest float64 to 1.005 sits a hair below it, so it rounds
		// to 1. Same half, opposite direction: binary, not decimal intent.
		{"float edge rounds to integer", 1.005, 1},
		{"exact value unchanged", 2.5, 2.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roundTo2d(tt.in); got != tt.want {
				t.Errorf("roundTo2d(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildCPU(t *testing.T) {
	t.Run("samples", func(t *testing.T) {
		got, avg, err := buildCPU([]float64{10.125, 20.5, 3.14159})
		if err != nil {
			t.Fatalf("buildCPU: %v", err)
		}
		want := map[string]float64{"C0": 10.13, "C1": 20.5, "C2": 3.14}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("map = %v, want %v", got, want)
		}
		// (10.125 + 20.5 + 3.14159) / 3 = 11.25553... → 11.26
		if avg != 11.26 {
			t.Errorf("avg = %v, want 11.26", avg)
		}
	})

	t.Run("empty", func(t *testing.T) {
		_, _, err := buildCPU(nil)
		if err == nil || err.Error() != "no per-cpu samples returned" {
			t.Errorf("err = %v, want \"no per-cpu samples returned\"", err)
		}
	})
}
