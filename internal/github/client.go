package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client is a GitHub API client with ETag caching and rate limit tracking.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
	etags      sync.Map // url → etag string
	cache      sync.Map // url → cached response body ([]byte)
	rateLimit  RateLimit
	mu         sync.RWMutex
}

// RateLimit holds the current GitHub API rate limit state.
type RateLimit struct {
	Remaining int
	Limit     int
	ResetAt   time.Time
}

// NewClient creates a Client targeting the real GitHub API.
func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      token,
		baseURL:    "https://api.github.com",
	}
}

// NewTestClient creates a client pointing to a test server URL.
func NewTestClient(token, baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		token:      token,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

// GetRateLimit returns the last observed rate limit values.
func (c *Client) GetRateLimit() RateLimit {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rateLimit
}

// get performs a GET with ETag caching. Returns (body, fromCache, error).
func (c *Client) get(ctx context.Context, path string) ([]byte, bool, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	if etag, ok := c.etags.Load(url); ok {
		req.Header.Set("If-None-Match", etag.(string))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	c.trackRateLimit(resp.Header)

	if resp.StatusCode == http.StatusNotModified {
		if cached, ok := c.cache.Load(url); ok {
			return cached.([]byte), true, nil
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, path)
	}

	if etag := resp.Header.Get("ETag"); etag != "" {
		c.etags.Store(url, etag)
		c.cache.Store(url, body)
	}

	return body, false, nil
}

// post performs a POST request.
func (c *Client) post(ctx context.Context, path string, body io.Reader) ([]byte, error) {
	return c.mutate(ctx, "POST", path, body)
}

// put performs a PUT request.
func (c *Client) put(ctx context.Context, path string, body io.Reader) ([]byte, error) {
	return c.mutate(ctx, "PUT", path, body)
}

// patch performs a PATCH request.
func (c *Client) patch(ctx context.Context, path string, body io.Reader) ([]byte, error) {
	return c.mutate(ctx, "PATCH", path, body)
}

func (c *Client) mutate(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	c.trackRateLimit(resp.Header)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GitHub API %s %d: %s", method, resp.StatusCode, path)
	}

	return respBody, nil
}

func (c *Client) trackRateLimit(h http.Header) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v := h.Get("X-RateLimit-Remaining"); v != "" {
		c.rateLimit.Remaining, _ = strconv.Atoi(v)
	}
	if v := h.Get("X-RateLimit-Limit"); v != "" {
		c.rateLimit.Limit, _ = strconv.Atoi(v)
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.rateLimit.ResetAt = time.Unix(ts, 0)
		}
	}
}

// getJSON performs a GET and unmarshals the response.
func (c *Client) getJSON(ctx context.Context, path string, v any) (bool, error) {
	body, fromCache, err := c.get(ctx, path)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fromCache, fmt.Errorf("parsing %s: %w", path, err)
	}
	return fromCache, nil
}
