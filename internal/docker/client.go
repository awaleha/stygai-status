package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Container struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Status string `json:"status"`
	Health string `json:"health"`
}

var ErrBadRequest = errors.New("bad request")
var ErrBadGateway = errors.New("invalid response from server")
var ErrInternal = errors.New("internal server error")
var ErrUnavailable = errors.New("service unavailable")

func List(ctx context.Context) ([]Container, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
			},
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://docker/containers/json?all=1", nil)
	if err != nil {
		return []Container{}, fmt.Errorf("creating request: %w, %w", ErrBadRequest, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return []Container{}, fmt.Errorf("executing request: %w, %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode > http.StatusMultipleChoices {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return []Container{}, fmt.Errorf("reading response: %w, %w", ErrInternal, err)
		}
		return []Container{}, fmt.Errorf("%w {status:%s, body:%s}", ErrBadGateway, resp.Status, string(body))
	}

	cList := []struct {
		Names  []string `json:"Names"`
		State  string   `json:"State"`
		Status string   `json:"Status"`
	}{}

	if err := json.NewDecoder(resp.Body).Decode(&cList); err != nil {
		return []Container{}, fmt.Errorf("decoding response: %w, %w", ErrInternal, err)
	}

	var containers []Container
	for _, c := range cList {
		var health string
		if strings.Contains(c.Status, "(healthy)") {
			health = "healthy"
		} else if strings.Contains(c.Status, "(unhealthy)") {
			health = "unhealthy"
		} else if strings.Contains(c.Status, "(health: starting)") {
			health = "starting"
		} else {
			health = ""
		}
		containers = append(containers, Container{
			Name:   strings.TrimPrefix(c.Names[0], "/"),
			State:  c.State,
			Status: c.Status,
			Health: health,
		})
	}

	sort.Slice(containers, func(i, j int) bool {
		return strings.ToLower(containers[i].Name) < strings.ToLower(containers[j].Name)
	})
	return containers, nil
}
