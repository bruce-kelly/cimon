package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"log/slog"
	"sync"
	"time"
)

// Client is a GitHub API client with ETag caching and rate limit tracking.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
	etagCache  *etagCache
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
		etagCache:  newETagCache(5000),
	}
}

// NewTestClient creates a client pointing to a test server URL.
func NewTestClient(token, baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		token:      token,
		baseURL:    strings.TrimRight(baseURL, "/"),
		etagCache:  newETagCache(5000),
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

	if etag, ok := c.etagCache.LoadETag(url); ok {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	c.trackRateLimit(resp.Header)

	if resp.StatusCode == http.StatusNotModified {
		if body, ok := c.etagCache.LoadBody(url); ok {
			return body, true, nil
		}
		return nil, false, fmt.Errorf("cache miss on 304 for %s", path)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, false, &AuthError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("GET %s", path),
		}
	}

	if resp.StatusCode == 429 {
		retryAfter := 60 * time.Second
		if v := resp.Header.Get("Retry-After"); v != "" {
			if secs, err := strconv.Atoi(v); err == nil {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		return nil, false, &RateLimitError{
			RetryAfter: retryAfter,
			ResetAt:    time.Now().Add(retryAfter),
		}
	}

	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, path)
	}

	if etag := resp.Header.Get("ETag"); etag != "" {
		c.etagCache.Store(url, etag, body)
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

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, &AuthError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("%s %s", method, path),
		}
	}

	if resp.StatusCode == 429 {
		retryAfter := 60 * time.Second
		if v := resp.Header.Get("Retry-After"); v != "" {
			if secs, err := strconv.Atoi(v); err == nil {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		return nil, &RateLimitError{
			RetryAfter: retryAfter,
			ResetAt:    time.Now().Add(retryAfter),
		}
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
		if remaining, err := strconv.Atoi(v); err == nil {
			c.rateLimit.Remaining = remaining
		} else {
			slog.Warn("failed to parse X-RateLimit-Remaining", "value", v, "err", err)
		}
	}
	if v := h.Get("X-RateLimit-Limit"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil {
			c.rateLimit.Limit = limit
		} else {
			slog.Warn("failed to parse X-RateLimit-Limit", "value", v, "err", err)
		}
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.rateLimit.ResetAt = time.Unix(ts, 0)
		} else {
			slog.Warn("failed to parse X-RateLimit-Reset", "value", v, "err", err)
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
