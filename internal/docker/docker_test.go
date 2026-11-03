package docker

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseList(t *testing.T) {
	fixture := `[
		{"Names":["/homepage"],"State":"running","Status":"Up 3 days (healthy)"},
		{"Names":["/traefik"],"State":"running","Status":"Up 3 days"},
		{"Names":["/Healthchecks"],"State":"running","Status":"Up 2 hours (unhealthy)"},
		{"Names":["/ntfy"],"State":"running","Status":"Up 2 hours (health: starting)"}
	]`
	// Names stripped of the leading slash and sorted case-insensitively;
	// every health form parsed, and a healthless Status tolerated.
	want := []Container{
		{Name: "Healthchecks", State: "running", Status: "Up 2 hours (unhealthy)", Health: "unhealthy"},
		{Name: "homepage", State: "running", Status: "Up 3 days (healthy)", Health: "healthy"},
		{Name: "ntfy", State: "running", Status: "Up 2 hours (health: starting)", Health: "starting"},
		{Name: "traefik", State: "running", Status: "Up 3 days", Health: ""},
	}
	got, err := parseList(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseList() = %+v, want %+v", got, want)
	}
}

func TestParseListBadJSON(t *testing.T) {
	_, err := parseList(strings.NewReader("not json"))
	if !errors.Is(err, ErrInternal) {
		t.Errorf("err = %v, want it to wrap ErrInternal", err)
	}
}
