package healthchecks

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Check struct {
	Name     string     `json:"name"`
	Status   string     `json:"status"`
	LastPing *time.Time `json:"last_ping"`
}

type Client struct {
	baseUrl string
	key     string
	http    *http.Client
}

func New(baseUrl, key string) *Client {
	c := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{ //To fix later, DEV ONLY
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	return &Client{baseUrl, key, c}
}

var ErrBadRequest = errors.New("bad request")
var ErrNotFound = errors.New("not found")
var ErrBadGateway = errors.New("invalid response from server")
var ErrInternal = errors.New("internal server error")
var ErrUnavailable = errors.New("service unavailable")

func (c *Client) Check(ctx context.Context, name string) (Check, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseUrl+"/api/v1/checks/", nil)
	if err != nil {
		return Check{}, fmt.Errorf("creating request: %w, %w", ErrBadRequest, err)
	}

	req.Header.Set("X-Api-Key", c.key)
	resp, err := c.http.Do(req)
	if err != nil {
		return Check{}, fmt.Errorf("executing request: %w, %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode > http.StatusMultipleChoices {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return Check{}, fmt.Errorf("reading response: %w, %w", ErrInternal, err)
		}
		return Check{}, fmt.Errorf("%w {status:%s, body:%s}", ErrBadGateway, resp.Status, string(body))
	}

	var wrapper struct {
		Checks []Check `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return Check{}, fmt.Errorf("decoding response: %w, %w", ErrInternal, err)
	}

	for _, c := range wrapper.Checks {
		if c.Name == name {
			return c, nil
		}
	}

	return Check{}, fmt.Errorf("healthcheck [%s] %w", name, ErrNotFound)
}
