package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL string, token string) *Client {
	c := &http.Client{
		Timeout: 10 * time.Second,
	}
	return &Client{baseURL, token, c}
}

var ErrBadGateway = errors.New("invalid response from server")
var ErrInternal = errors.New("internal server error")
var ErrUnavailable = errors.New("service unavailable")

func (c *Client) Publish(ctx context.Context, topic, message string, tags ...string) (string, error) {
	body, _ := json.Marshal(map[string]any{"topic": topic, "message": message, "tags": tags})
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w, %w", ErrInternal, err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w, %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode > http.StatusMultipleChoices {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("reading response: %w, %w", ErrInternal, err)
		}
		return "", fmt.Errorf("%w {status:%s, body:%s}", ErrBadGateway, resp.Status, string(body))
	}
	var id struct {
		Id string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&id); err != nil {
		return "", fmt.Errorf("reading response: %w, %w", ErrInternal, err)
	}
	return id.Id, nil
}
