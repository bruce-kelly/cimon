# CIMON Audit Remediation — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Transform CIMON from a read-only CI monitor into a fully interactive control plane by wiring all disconnected components, hardening error paths, and integrating agent infrastructure.

**Architecture:** The existing code is well-structured: GitHub client, DB, poller, UI components, and agent infrastructure are all implemented and tested individually. The gap is in `internal/app/app.go` which needs to instantiate and wire these pieces together. Most tasks modify `app.go` plus the component being connected. Tasks are designed to be independent and parallelizable.

**Tech Stack:** Go 1.25, Bubbletea v2, Lipgloss v2, modernc.org/sqlite, robfig/cron/v3. Tests use testify/assert and httptest.

**Build/Test:** `export PATH=$HOME/go-install/go/bin:$PATH` before any `go` commands.

---

## Phase 1: Foundation Hardening

These tasks fix critical defects that cause crashes, memory leaks, or silent data loss. No visible feature changes — just correctness. All tasks in this phase are independent and can run in parallel.

---

### Task 1: Bounded ETag Cache

The `sync.Map` in `internal/github/client.go` stores ETags and response bodies indefinitely per URL. With multi-repo polling over hours, this grows without bound. Replace with a bounded LRU cache.

**Files:**
- Create: `internal/github/cache.go`
- Create: `internal/github/cache_test.go`
- Modify: `internal/github/client.go:16-24` (Client struct), `client.go:59-100` (get method)
- Modify: `internal/github/client_test.go` (existing ETag tests should still pass)

**Step 1: Write tests for the bounded cache**

Create `internal/github/cache_test.go`:

```go
package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestETagCache_StoreAndLoad(t *testing.T) {
	c := newETagCache(100)
	c.Store("/repos/foo/runs", "etag-1", []byte(`{"runs":[]}`))

	etag, ok := c.LoadETag("/repos/foo/runs")
	assert.True(t, ok)
	assert.Equal(t, "etag-1", etag)

	body, ok := c.LoadBody("/repos/foo/runs")
	assert.True(t, ok)
	assert.Equal(t, []byte(`{"runs":[]}`), body)
}

func TestETagCache_MissReturnsNotOK(t *testing.T) {
	c := newETagCache(100)

	_, ok := c.LoadETag("/repos/foo/runs")
	assert.False(t, ok)

	_, ok = c.LoadBody("/repos/foo/runs")
	assert.False(t, ok)
}

func TestETagCache_EvictsOldest(t *testing.T) {
	c := newETagCache(3)

	c.Store("/a", "etag-a", []byte("a"))
	c.Store("/b", "etag-b", []byte("b"))
	c.Store("/c", "etag-c", []byte("c"))

	// All three present
	_, ok := c.LoadETag("/a")
	assert.True(t, ok)

	// Adding a 4th evicts the oldest (/a)
	c.Store("/d", "etag-d", []byte("d"))

	_, ok = c.LoadETag("/a")
	assert.False(t, ok, "oldest entry should be evicted")

	_, ok = c.LoadETag("/d")
	assert.True(t, ok, "newest entry should be present")
}

func TestETagCache_UpdateRefreshesPosition(t *testing.T) {
	c := newETagCache(3)

	c.Store("/a", "etag-a", []byte("a"))
	c.Store("/b", "etag-b", []byte("b"))
	c.Store("/c", "etag-c", []byte("c"))

	// Access /a to refresh it
	c.Store("/a", "etag-a2", []byte("a2"))

	// Adding /d should evict /b (oldest non-refreshed), not /a
	c.Store("/d", "etag-d", []byte("d"))

	_, ok := c.LoadETag("/a")
	assert.True(t, ok, "/a was refreshed, should survive")

	_, ok = c.LoadETag("/b")
	assert.False(t, ok, "/b should be evicted")
}

func TestETagCache_Delete(t *testing.T) {
	c := newETagCache(100)
	c.Store("/a", "etag-a", []byte("a"))

	c.Delete("/a")
	_, ok := c.LoadETag("/a")
	assert.False(t, ok)
}

func TestETagCache_Len(t *testing.T) {
	c := newETagCache(100)
	assert.Equal(t, 0, c.Len())

	c.Store("/a", "e", []byte("a"))
	assert.Equal(t, 1, c.Len())

	c.Store("/b", "e", []byte("b"))
	assert.Equal(t, 2, c.Len())

	c.Delete("/a")
	assert.Equal(t, 1, c.Len())
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/github/ -run TestETagCache -v`
Expected: Compilation error — `newETagCache` undefined.

**Step 3: Implement the bounded cache**

Create `internal/github/cache.go`:

```go
package github

import (
	"container/list"
	"sync"
)

// etagEntry holds a cached response with its ETag.
type etagEntry struct {
	url   string
	etag  string
	body  []byte
	elem  *list.Element
}

// etagCache is a bounded LRU cache for HTTP ETags and response bodies.
// Thread-safe via mutex.
type etagCache struct {
	mu      sync.Mutex
	items   map[string]*etagEntry
	order   *list.List // front = newest, back = oldest
	maxSize int
}

func newETagCache(maxSize int) *etagCache {
	return &etagCache{
		items:   make(map[string]*etagEntry),
		order:   list.New(),
		maxSize: maxSize,
	}
}

// Store adds or updates a cache entry. Evicts oldest if at capacity.
func (c *etagCache) Store(url, etag string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.items[url]; ok {
		// Update existing: refresh position
		entry.etag = etag
		entry.body = body
		c.order.MoveToFront(entry.elem)
		return
	}

	// Evict oldest if at capacity
	for c.order.Len() >= c.maxSize {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		old := oldest.Value.(*etagEntry)
		c.order.Remove(oldest)
		delete(c.items, old.url)
	}

	// Insert new entry
	entry := &etagEntry{url: url, etag: etag, body: body}
	entry.elem = c.order.PushFront(entry)
	c.items[url] = entry
}

// LoadETag returns the cached ETag for a URL.
func (c *etagCache) LoadETag(url string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[url]
	if !ok {
		return "", false
	}
	return entry.etag, true
}

// LoadBody returns the cached response body for a URL.
func (c *etagCache) LoadBody(url string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[url]
	if !ok {
		return nil, false
	}
	return entry.body, true
}

// Delete removes a cache entry.
func (c *etagCache) Delete(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.items[url]; ok {
		c.order.Remove(entry.elem)
		delete(c.items, url)
	}
}

// Len returns the number of cached entries.
func (c *etagCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
```

**Step 4: Run cache tests to verify they pass**

Run: `go test ./internal/github/ -run TestETagCache -v`
Expected: All 7 tests PASS.

**Step 5: Replace sync.Map with etagCache in Client**

In `internal/github/client.go`, change the Client struct (lines 16-24):

```go
// Before:
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
	etags      sync.Map
	cache      sync.Map
	rateLimit  RateLimit
	mu         sync.RWMutex
}

// After:
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
	etagCache  *etagCache
	rateLimit  RateLimit
	mu         sync.RWMutex
}
```

Update `NewClient()` (lines 34-40):

```go
func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      token,
		baseURL:    "https://api.github.com",
		etagCache:  newETagCache(5000),
	}
}
```

Update `NewTestClient()` (lines 43-49) similarly, using `newETagCache(5000)`.

Update `get()` method (lines 59-100). Replace the sync.Map calls:

```go
// Line 68-70 — was: c.etags.Load(url)
if etag, ok := c.etagCache.LoadETag(url); ok {
	req.Header.Set("If-None-Match", etag)
}

// Lines 79-82 — was: c.cache.Load(url)
if resp.StatusCode == http.StatusNotModified {
	if body, ok := c.etagCache.LoadBody(url); ok {
		return body, true, nil
	}
	return nil, false, fmt.Errorf("cache miss on 304 for %s", path)
}

// Lines 94-97 — was: c.etags.Store(url, ...) + c.cache.Store(url, ...)
if etag := resp.Header.Get("ETag"); etag != "" {
	c.etagCache.Store(url, etag, body)
}
```

Remove the `"sync"` import if no longer needed (check if `sync.RWMutex` still used — yes, for `rateLimit`).

**Step 6: Run all GitHub client tests**

Run: `go test ./internal/github/ -v -count=1`
Expected: All existing tests PASS (ETag behavior unchanged, just bounded).

**Step 7: Run full test suite**

Run: `go test ./... -count=1`
Expected: All 139+ tests PASS.

**Step 8: Commit**

```bash
git add internal/github/cache.go internal/github/cache_test.go internal/github/client.go
git commit -m "fix: replace unbounded sync.Map ETag cache with bounded LRU (5000 entries)"
```

---

### Task 2: Fix Poller Channel Lifecycle

The poller sends to `resultCh` without checking `ctx.Done()`, causing potential deadlock on quit. The `waitForPoll` function returns nil on channel close, silently freezing the TUI.

**Files:**
- Modify: `internal/polling/poller.go:64-80` (pollOnce — add select with ctx.Done)
- Modify: `internal/app/app.go:144-158` (waitForPoll — detect channel close, add timeout)
- Modify: `internal/app/app.go:160-183` (Update — handle PollErrorMsg)
- Modify: `internal/app/app.go:343-347` (quit — close channel before cancel)
- Modify: `internal/polling/polling_test.go` (add channel lifecycle tests)

**Step 1: Write test for poller context cancellation**

Add to `internal/polling/polling_test.go`:

```go
func TestPoller_StopClosesCleanly(t *testing.T) {
	// Verify that stopping the poller doesn't leave goroutines hanging
	cfg := &config.CimonConfig{
		Repos: []config.RepoConfig{
			{Repo: "owner/repo", Branch: "main"},
		},
		Polling: config.PollingConfig{Idle: 30, Active: 5, Cooldown: 3},
	}

	resultCh := make(chan models.PollResult, 1)
	client := github.NewTestClient("fake-token", "http://localhost:1")
	p := New(client, cfg, resultCh)

	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context — should not hang
	cancel()
	p.Stop()

	// Channel should be drainable without blocking
	done := make(chan bool, 1)
	go func() {
		for range resultCh {
		}
		done <- true
	}()
	close(resultCh)

	select {
	case <-done:
		// Clean exit
	case <-time.After(2 * time.Second):
		t.Fatal("channel drain timed out — goroutine leak")
	}
}
```

**Step 2: Run test to verify behavior**

Run: `go test ./internal/polling/ -run TestPoller_StopClosesCleanly -v`
Expected: May pass or hang depending on timing. The fix ensures it always passes.

**Step 3: Fix poller to check ctx.Done before sending**

In `internal/polling/poller.go`, modify `pollOnce()` (lines 64-80). Change the channel send (line 74) from:

```go
p.resultCh <- result
```

To:

```go
select {
case p.resultCh <- result:
case <-ctx.Done():
	return
}
```

Also add a `ctx.Done()` check at the top of `pollOnce()`:

```go
func (p *Poller) pollOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	// ... existing code ...
}
```

And in `loop()` (lines 52-62), add ctx check in the timer select:

```go
func (p *Poller) loop(ctx context.Context) {
	p.pollOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.state.Interval()):
			p.pollOnce(ctx)
		}
	}
}
```

**Step 4: Fix waitForPoll to detect channel close**

In `internal/app/app.go`, add a new message type and modify `waitForPoll`:

```go
// Add near pollResultMsg (around line 30):
type pollErrorMsg struct {
	Err error
}

// Replace waitForPoll (lines 150-158):
func waitForPoll(ch <-chan models.PollResult) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return pollErrorMsg{Err: fmt.Errorf("poll channel closed")}
		}
		return pollResultMsg{Result: result}
	}
}
```

**Step 5: Handle pollErrorMsg in Update**

In `internal/app/app.go`, add a case in `Update()` (after line 163):

```go
case pollErrorMsg:
	a.statusText = fmt.Sprintf("Poll error: %v", msg.Err)
	// Don't re-subscribe — channel is closed
	return a, nil
```

**Step 6: Close channel on quit**

In `internal/app/app.go`, modify quit handling (lines 343-347):

```go
case key.Matches(msg, ui.Keys.Quit):
	a.quitting = true
	a.poller.Stop()
	a.cancel()
	return a, tea.Quit
```

Note: `poller.Stop()` is called before `cancel()` so the poller's loop exits via its own cancel, then the context cancel cleans up anything else.

**Step 7: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 8: Commit**

```bash
git add internal/polling/poller.go internal/polling/polling_test.go internal/app/app.go
git commit -m "fix: prevent goroutine leak and deadlock on quit — poller checks ctx.Done before send"
```

---

### Task 3: HTTP Error Classification (Auth + Rate Limit)

GitHub 401/403 and 429 responses are treated as generic errors. Add specific error types so the app can react appropriately (show auth failure message, backoff on rate limit).

**Files:**
- Create: `internal/github/errors.go`
- Create: `internal/github/errors_test.go`
- Modify: `internal/github/client.go:85-100` (get method — classify errors)
- Modify: `internal/github/client.go:117-146` (mutate method — classify errors)
- Modify: `internal/github/client_test.go` (add 401/429 tests)
- Modify: `internal/app/app.go` (handle classified errors in status bar)
- Modify: `internal/models/poll.go` (add fields for error classification)

**Step 1: Write tests for error types**

Create `internal/github/errors_test.go`:

```go
package github

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAuthError_Is(t *testing.T) {
	err := &AuthError{StatusCode: 401, Message: "bad token"}
	assert.True(t, errors.As(err, &AuthError{}))
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestRateLimitError_RetryAfter(t *testing.T) {
	err := &RateLimitError{
		RetryAfter: 60 * time.Second,
		ResetAt:    time.Now().Add(60 * time.Second),
	}
	assert.True(t, errors.As(err, &RateLimitError{}))
	assert.Contains(t, err.Error(), "rate limited")
}
```

**Step 2: Run tests — expect compilation failure**

Run: `go test ./internal/github/ -run "TestAuthError|TestRateLimitError" -v`
Expected: Compilation error — types not defined.

**Step 3: Implement error types**

Create `internal/github/errors.go`:

```go
package github

import (
	"fmt"
	"time"
)

// AuthError indicates the GitHub token is invalid, expired, or lacks permissions.
type AuthError struct {
	StatusCode int
	Message    string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("authentication failed (HTTP %d): %s", e.StatusCode, e.Message)
}

// RateLimitError indicates GitHub rate limiting (HTTP 429 or X-RateLimit-Remaining: 0).
type RateLimitError struct {
	RetryAfter time.Duration
	ResetAt    time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited; retry after %v (resets at %s)",
		e.RetryAfter.Round(time.Second), e.ResetAt.Format("15:04:05"))
}
```

**Step 4: Run error type tests**

Run: `go test ./internal/github/ -run "TestAuthError|TestRateLimitError" -v`
Expected: PASS.

**Step 5: Add httptest tests for 401 and 429 handling**

Add to `internal/github/client_test.go`:

```go
func TestClient_Get401ReturnsAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	c := NewTestClient("bad-token", srv.URL)
	_, _, err := c.get(context.Background(), "/repos/owner/repo")
	assert.Error(t, err)

	var authErr *AuthError
	assert.True(t, errors.As(err, &authErr))
	assert.Equal(t, 401, authErr.StatusCode)
}

func TestClient_Get429ReturnsRateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
		w.Write([]byte(`{"message":"rate limit exceeded"}`))
	}))
	defer srv.Close()

	c := NewTestClient("token", srv.URL)
	_, _, err := c.get(context.Background(), "/repos/owner/repo")
	assert.Error(t, err)

	var rlErr *RateLimitError
	assert.True(t, errors.As(err, &rlErr))
	assert.Equal(t, 120*time.Second, rlErr.RetryAfter)
}

func TestClient_Get429DefaultsRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer srv.Close()

	c := NewTestClient("token", srv.URL)
	_, _, err := c.get(context.Background(), "/repos/owner/repo")

	var rlErr *RateLimitError
	assert.True(t, errors.As(err, &rlErr))
	assert.Equal(t, 60*time.Second, rlErr.RetryAfter)
}
```

**Step 6: Run new tests — expect failure**

Run: `go test ./internal/github/ -run "TestClient_Get401|TestClient_Get429" -v`
Expected: FAIL — get() returns generic error, not AuthError/RateLimitError.

**Step 7: Update get() to classify errors**

In `internal/github/client.go`, replace the error handling block (around lines 85-92). Before:

```go
if resp.StatusCode >= 400 {
	return nil, false, fmt.Errorf("GitHub API %d: %s %s", resp.StatusCode, method, path)
}
```

After:

```go
if resp.StatusCode == 401 || resp.StatusCode == 403 {
	return nil, false, &AuthError{
		StatusCode: resp.StatusCode,
		Message:    fmt.Sprintf("%s %s", resp.Method, path),
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
```

Add `"strconv"` to imports if not already present.

Apply the same 401/403 and 429 checks in `mutate()` (lines 117-146) — same pattern, before the generic `>= 400` check.

**Step 8: Run all GitHub client tests**

Run: `go test ./internal/github/ -v -count=1`
Expected: All tests PASS (new + existing).

**Step 9: Surface auth/rate-limit errors in App status bar**

In `internal/app/app.go`, modify `handlePollResult()` to check for classified errors. The PollResult already has an `Error` field. In `internal/polling/poller.go`, errors from API calls are currently logged but not propagated. We need to propagate them.

In `internal/polling/poller.go`, modify `pollRepo()` (around line 92) to set the error on the result:

```go
// After ListRuns errors, instead of just logging, also set:
if err != nil {
	slog.Error("list runs", "repo", repo.Repo, "workflow", wf, "err", err)
	result.Error = err
	// Continue to collect partial results
}
```

Then in `internal/app/app.go`, in `handlePollResult()`, add error checking before the existing logic:

```go
func (a App) handlePollResult(msg pollResultMsg) (tea.Model, tea.Cmd) {
	// Check for classified errors
	if msg.Result.Error != nil {
		var authErr *ghclient.AuthError
		var rlErr *ghclient.RateLimitError
		switch {
		case errors.As(msg.Result.Error, &authErr):
			a.statusText = "AUTH FAILED — token expired or revoked. Restart with valid token."
			return a, waitForPoll(a.resultCh)
		case errors.As(msg.Result.Error, &rlErr):
			a.statusText = fmt.Sprintf("Rate limited — retrying at %s",
				rlErr.ResetAt.Format("15:04:05"))
			return a, waitForPoll(a.resultCh)
		}
	}

	// ... existing logic (repo := msg.Result.Repo, etc.) ...
```

Add `"errors"` and the github client import alias to app.go imports.

**Step 10: Run full test suite**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 11: Commit**

```bash
git add internal/github/errors.go internal/github/errors_test.go \
       internal/github/client.go internal/github/client_test.go \
       internal/polling/poller.go internal/app/app.go
git commit -m "fix: classify 401/403 as AuthError and 429 as RateLimitError with backoff"
```

---

### Task 4: SQLite Safety (WAL Checkpoint + Rate Limit Parse Logging)

Fix WAL checkpoint gap between schema creation and reader connection. Add checkpoint after prune. Log rate limit parse errors instead of silently ignoring them.

**Files:**
- Modify: `internal/db/db.go:26-56` (Open — add checkpoint after schema exec)
- Modify: `internal/db/runs.go` (PruneRuns — add checkpoint after prune, if PruneRuns exists there)
- Modify: `internal/github/client.go:148-162` (trackRateLimit — log parse errors)

**Step 1: Add WAL checkpoint after schema in Open()**

In `internal/db/db.go`, after the schema exec (around line 44) and before opening the reader:

```go
// Force WAL checkpoint so reader sees schema
if _, err := writer.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
	writer.Close()
	return nil, fmt.Errorf("wal checkpoint: %w", err)
}
```

**Step 2: Add WAL checkpoint after prune**

Find the prune function. Check if it's in `internal/db/runs.go` or `cmd/cimon/db.go`. Add after the DELETE commit:

```go
// Reclaim WAL space after bulk delete
if _, err := d.writer.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
	slog.Warn("WAL checkpoint after prune failed", "err", err)
}
```

**Step 3: Fix rate limit header parsing**

In `internal/github/client.go`, replace the `trackRateLimit` function (lines 148-162). Change from ignoring `strconv.Atoi` errors to logging them:

```go
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
```

**Step 4: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 5: Commit**

```bash
git add internal/db/db.go internal/github/client.go
git commit -m "fix: WAL checkpoint after schema/prune, log rate limit parse errors"
```

---

### Task 5: Fix Silent Data Corruption (Timestamp Parsing + Type Assertions)

`time.Parse` errors are silently swallowed in GitHub response parsing. `sync.Map` type assertions (now `etagCache`) should be safe, but the timestamp issue needs fixing.

**Files:**
- Modify: `internal/github/pulls.go` (toModel — handle time.Parse errors)
- Modify: `internal/github/runs.go` (toModel — handle time.Parse errors)
- Modify: `internal/github/actions.go` (handle json.Marshal errors)

**Step 1: Fix timestamp parsing in pulls.go**

Find all `time.Parse(time.RFC3339, ...)` calls that use `_` for the error. Replace with:

```go
created, err := time.Parse(time.RFC3339, p.CreatedAt)
if err != nil {
	slog.Warn("failed to parse PR created_at", "value", p.CreatedAt, "err", err)
	created = time.Now()
}
```

Apply the same pattern for `UpdatedAt`.

**Step 2: Fix timestamp parsing in runs.go**

Same pattern for `CreatedAt` and `UpdatedAt` in the run response conversion.

**Step 3: Fix json.Marshal in actions.go**

Replace all `body, _ := json.Marshal(...)` with proper error handling:

```go
body, err := json.Marshal(map[string]string{"event": "APPROVE"})
if err != nil {
	return fmt.Errorf("marshaling request body: %w", err)
}
```

Apply to all 5 action methods (Approve, Merge, Rerun, RerunFailed, Cancel — or whichever exist).

**Step 4: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 5: Commit**

```bash
git add internal/github/pulls.go internal/github/runs.go internal/github/actions.go
git commit -m "fix: handle timestamp parse errors and json.Marshal errors instead of swallowing"
```

---

## Phase 2: Action Wiring

These tasks wire all the existing but disconnected interactive features. Each task adds one capability to `internal/app/app.go`. All tasks in this phase depend on Phase 1 being complete but are independent of each other within the phase, EXCEPT: Task 6 (Flash + ConfirmBar) should be done first as other tasks reference these components.

---

### Task 6: Wire Flash and ConfirmBar to App

Add Flash (success/error messages) and ConfirmBar (y/n confirmation) to the App. These are prerequisites for all action commands.

**Files:**
- Modify: `internal/app/app.go` (add fields, init, render in View, handle in Update)

**Step 1: Add fields to App struct**

In `internal/app/app.go`, add to the App struct (around line 50, in the "Overlays" section):

```go
// Overlays
help       components.HelpOverlay
flash      components.Flash
confirmBar components.ConfirmBar
```

**Step 2: Wire ConfirmBar key handling in Update**

In `Update()`, add a case before the `tea.KeyPressMsg` handler (around line 178) to intercept keys when ConfirmBar is active:

```go
case tea.KeyPressMsg:
	// ConfirmBar intercepts all keys when active
	if a.confirmBar.Active {
		handled := a.confirmBar.HandleKey(msg.String())
		if handled {
			return a, nil
		}
	}
	return a.handleKey(msg)
```

**Step 3: Render Flash and ConfirmBar in View**

In `View()` (around line 500), modify the bottom bar rendering. Currently the bottom bar shows `a.statusText`. Add Flash overlay and ConfirmBar:

```go
// Bottom bar — show confirmBar if active, flash if visible, else status
var bottomContent string
if a.confirmBar.Active {
	bottomContent = a.confirmBar.Render(a.width)
} else if a.flash.Visible() {
	style := lipgloss.NewStyle().Width(a.width)
	if a.flash.IsError {
		style = style.Foreground(ui.ColorRed)
	} else {
		style = style.Foreground(ui.ColorGreen)
	}
	bottomContent = style.Render(" " + a.flash.Message)
} else {
	bottomContent = statusBarStyle.Render(a.statusText)
}
```

Use `bottomContent` where the status bar was previously rendered.

**Step 4: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 5: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire Flash and ConfirmBar overlays to App — prerequisites for actions"
```

---

### Task 7: Wire Pipeline Actions (Rerun, Open Browser)

Wire the `r` (rerun) and `o` (open in browser) keys for workflow runs on the dashboard.

**Files:**
- Modify: `internal/app/app.go:389-413` (handleDashboardKey — add cases)

**Step 1: Add rerun handler**

In `handleDashboardKey()`, add:

```go
case key.Matches(msg, ui.Keys.Rerun):
	if a.dashboard.Focus == screens.FocusPipeline {
		if run := a.dashboard.Pipeline.SelectedRun(); run != nil {
			a.confirmBar.Show(
				fmt.Sprintf("Rerun %s #%d?", run.Name, run.ID),
				func() {
					go func() {
						var err error
						if run.Conclusion == "failure" {
							err = a.client.RerunFailed(a.ctx, run.Repo, run.ID)
						} else {
							err = a.client.Rerun(a.ctx, run.Repo, run.ID)
						}
						if err != nil {
							a.flash.Show("Rerun failed: "+err.Error(), true)
						} else {
							a.flash.Show("Rerun triggered", false)
						}
					}()
				},
				func() {},
			)
		}
	}
```

**Step 2: Add open-in-browser handler**

```go
case key.Matches(msg, ui.Keys.Open):
	var url string
	switch a.dashboard.Focus {
	case screens.FocusPipeline:
		if run := a.dashboard.Pipeline.SelectedRun(); run != nil {
			url = run.HTMLURL
		}
	case screens.FocusReview:
		idx := a.dashboard.ReviewSel.Index()
		if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
			url = a.dashboard.ReviewItems[idx].PR.HTMLURL
		}
	}
	if url != "" {
		go openBrowser(url)
	}
```

Add the `openBrowser` helper function at the bottom of `app.go`:

```go
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		cmd.Run()
	}
}
```

Add `"os/exec"` and `"runtime"` to imports.

**Step 3: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire rerun (r) and open-in-browser (o) actions on dashboard"
```

---

### Task 8: Wire Review Queue Actions (Approve, Merge, Dismiss)

Wire `a` (approve), `m` (merge), `M` (batch merge agent PRs), and `x` (dismiss) for PRs in the review queue.

**Files:**
- Modify: `internal/app/app.go` (App struct — add dismissed field, handleDashboardKey — add cases, rebuildScreenData — pass dismissed, NewApp — load dismissed)

**Step 1: Add dismissed field and load on startup**

In `internal/app/app.go`, add to App struct:

```go
dismissed map[string]bool
```

In `NewApp()`, after DB is available, load dismissed items:

```go
dismissed, err := database.LoadDismissed()
if err != nil {
	slog.Error("loading dismissed items", "err", err)
	dismissed = make(map[string]bool)
}
```

Set `a.dismissed = dismissed` on the App.

**Step 2: Pass dismissed to ReviewItemsFromPulls**

In `rebuildScreenData()` (around line 248), change the nil to `a.dismissed`:

```go
a.dashboard.ReviewItems = review.ReviewItemsFromPulls(
	allPulls,
	a.config.ReviewQueue.Escalation.Amber,
	a.config.ReviewQueue.Escalation.Red,
	a.dismissed,
)
```

**Step 3: Add approve handler**

In `handleDashboardKey()`:

```go
case key.Matches(msg, ui.Keys.Approve):
	if a.dashboard.Focus == screens.FocusReview {
		idx := a.dashboard.ReviewSel.Index()
		if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
			pr := a.dashboard.ReviewItems[idx].PR
			a.confirmBar.Show(
				fmt.Sprintf("Approve %s#%d?", pr.Repo, pr.Number),
				func() {
					go func() {
						if err := a.client.Approve(a.ctx, pr.Repo, pr.Number); err != nil {
							a.flash.Show("Approve failed: "+err.Error(), true)
						} else {
							a.flash.Show(fmt.Sprintf("Approved %s#%d", pr.Repo, pr.Number), false)
						}
					}()
				},
				func() {},
			)
		}
	}
```

**Step 4: Add merge handler**

```go
case key.Matches(msg, ui.Keys.Merge):
	if a.dashboard.Focus == screens.FocusReview {
		idx := a.dashboard.ReviewSel.Index()
		if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
			pr := a.dashboard.ReviewItems[idx].PR
			a.confirmBar.Show(
				fmt.Sprintf("Merge %s#%d?", pr.Repo, pr.Number),
				func() {
					go func() {
						if err := a.client.Merge(a.ctx, pr.Repo, pr.Number); err != nil {
							a.flash.Show("Merge failed: "+err.Error(), true)
						} else {
							a.flash.Show(fmt.Sprintf("Merged %s#%d", pr.Repo, pr.Number), false)
						}
					}()
				},
				func() {},
			)
		}
	}
```

**Step 5: Add batch merge handler**

```go
case key.Matches(msg, ui.Keys.BatchMerge):
	var agentReady []models.PullRequest
	for _, item := range a.dashboard.ReviewItems {
		if item.PR.IsAgent && item.PR.CIStatus == "success" && item.PR.ReviewState == "approved" {
			agentReady = append(agentReady, item.PR)
		}
	}
	if len(agentReady) > 0 {
		a.confirmBar.Show(
			fmt.Sprintf("Batch merge %d agent PRs?", len(agentReady)),
			func() {
				go func() {
					merged := 0
					for _, pr := range agentReady {
						if err := a.client.Merge(a.ctx, pr.Repo, pr.Number); err != nil {
							slog.Error("batch merge failed", "pr", pr.Number, "err", err)
						} else {
							merged++
						}
					}
					a.flash.Show(fmt.Sprintf("Merged %d/%d agent PRs", merged, len(agentReady)), merged < len(agentReady))
				}()
			},
			func() {},
		)
	}
```

**Step 6: Add dismiss handler**

```go
case key.Matches(msg, ui.Keys.Dismiss):
	if a.dashboard.Focus == screens.FocusReview {
		idx := a.dashboard.ReviewSel.Index()
		if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
			pr := a.dashboard.ReviewItems[idx].PR
			key := fmt.Sprintf("%s:%d", pr.Repo, pr.Number)
			if err := a.db.AddDismissed(pr.Repo, pr.Number); err != nil {
				a.flash.Show("Dismiss failed: "+err.Error(), true)
			} else {
				a.dismissed[key] = true
				a.rebuildScreenData()
				a.flash.Show(fmt.Sprintf("Dismissed %s#%d", pr.Repo, pr.Number), false)
			}
		}
	}
```

**Step 7: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 8: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire approve (a), merge (m), batch merge (M), dismiss (x) actions"
```

---

### Task 9: Wire LogPane (Diff Viewing)

Connect the LogPane component to show PR diffs and CI run links.

**Files:**
- Modify: `internal/app/app.go` (add logPane field, wire `v` and `l` keys, render in View)

**Step 1: Add LogPane field to App**

In the overlays section of the App struct:

```go
logPane components.LogPane
```

**Step 2: Wire `l` key (cycle log pane mode) in handleKey**

In `handleKey()` (lines 316-387), add a case that works on all screens:

```go
case key.Matches(msg, ui.Keys.LogCycle):
	a.logPane.CycleMode()
	return a, nil
```

**Step 3: Wire `v` key (view diff) in handleDashboardKey**

```go
case key.Matches(msg, ui.Keys.ViewDiff):
	switch a.dashboard.Focus {
	case screens.FocusReview:
		idx := a.dashboard.ReviewSel.Index()
		if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
			pr := a.dashboard.ReviewItems[idx].PR
			go func() {
				diff, err := a.client.GetPullDiff(a.ctx, pr.Repo, pr.Number)
				if err != nil {
					a.flash.Show("Failed to fetch diff: "+err.Error(), true)
					return
				}
				a.logPane.SetContent(
					fmt.Sprintf("%s#%d — %s", pr.Repo, pr.Number, pr.Title),
					diff,
					false,
				)
				if a.logPane.Mode == components.LogPaneHidden {
					a.logPane.CycleMode()
				}
			}()
		}
	case screens.FocusPipeline:
		if run := a.dashboard.Pipeline.SelectedRun(); run != nil {
			a.logPane.SetContent(
				fmt.Sprintf("%s — %s", run.Name, run.HTMLURL),
				fmt.Sprintf("Run #%d\nStatus: %s\nConclusion: %s\nBranch: %s\nActor: %s\n",
					run.ID, run.Status, run.Conclusion, run.HeadBranch, run.Actor),
				false,
			)
			if a.logPane.Mode == components.LogPaneHidden {
				a.logPane.CycleMode()
			}
		}
	}
```

**Step 4: Render LogPane in View**

In `View()`, when LogPane is not hidden, split the content area. After rendering the screen content but before composing the final view:

```go
// Render log pane if visible
logContent := ""
if a.logPane.Mode != components.LogPaneHidden {
	logContent = a.logPane.Render(a.width, contentHeight)
}

// Compose: top bar + screen content (+ log pane) + bottom bar
var rendered string
if logContent != "" {
	// Split content area between screen and log pane
	logHeight := contentHeight / 2
	if a.logPane.Mode == components.LogPaneFull {
		logHeight = contentHeight
	}
	screenHeight := contentHeight - logHeight

	// Truncate screen content to screenHeight lines
	screenLines := strings.Split(content, "\n")
	if len(screenLines) > screenHeight {
		screenLines = screenLines[:screenHeight]
	}
	content = strings.Join(screenLines, "\n")

	rendered = topBar + "\n" + content + "\n" + logContent + "\n" + bottomContent
} else {
	rendered = topBar + "\n" + content + "\n" + bottomContent
}
```

Adjust as needed to fit the existing View() rendering approach.

**Step 5: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 6: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire LogPane — view PR diffs (v) and cycle log pane mode (l)"
```

---

### Task 10: Wire ActionMenu (Enter Key)

Wire the ActionMenu component to show contextual actions when Enter is pressed.

**Files:**
- Modify: `internal/app/app.go` (add actionMenu field, wire Enter key, render overlay)

**Step 1: Add ActionMenu field to App**

```go
actionMenu components.ActionMenu
```

**Step 2: Wire Enter key in handleDashboardKey**

```go
case key.Matches(msg, ui.Keys.Enter):
	// Build context-sensitive action menu
	var items []components.ActionMenuItem

	switch a.dashboard.Focus {
	case screens.FocusPipeline:
		if run := a.dashboard.Pipeline.SelectedRun(); run != nil {
			items = append(items, components.ActionMenuItem{
				Label: "Rerun all jobs", Key: "r",
				Action: func() {
					go func() {
						if err := a.client.Rerun(a.ctx, run.Repo, run.ID); err != nil {
							a.flash.Show("Rerun failed: "+err.Error(), true)
						} else {
							a.flash.Show("Rerun triggered", false)
						}
					}()
				},
			})
			if run.Conclusion == "failure" {
				items = append(items, components.ActionMenuItem{
					Label: "Rerun failed jobs", Key: "f",
					Action: func() {
						go func() {
							if err := a.client.RerunFailed(a.ctx, run.Repo, run.ID); err != nil {
								a.flash.Show("Rerun failed: "+err.Error(), true)
							} else {
								a.flash.Show("Rerun (failed only) triggered", false)
							}
						}()
					},
				})
			}
			if run.IsActive() {
				items = append(items, components.ActionMenuItem{
					Label: "Cancel", Key: "c",
					Action: func() {
						go func() {
							if err := a.client.Cancel(a.ctx, run.Repo, run.ID); err != nil {
								a.flash.Show("Cancel failed: "+err.Error(), true)
							} else {
								a.flash.Show("Run cancelled", false)
							}
						}()
					},
				})
			}
			items = append(items, components.ActionMenuItem{
				Label: "Open in browser", Key: "o",
				Action: func() { go openBrowser(run.HTMLURL) },
			})
		}

	case screens.FocusReview:
		idx := a.dashboard.ReviewSel.Index()
		if idx >= 0 && idx < len(a.dashboard.ReviewItems) {
			pr := a.dashboard.ReviewItems[idx].PR
			items = append(items,
				components.ActionMenuItem{
					Label: "Approve", Key: "a",
					Action: func() {
						go func() {
							if err := a.client.Approve(a.ctx, pr.Repo, pr.Number); err != nil {
								a.flash.Show("Approve failed: "+err.Error(), true)
							} else {
								a.flash.Show(fmt.Sprintf("Approved %s#%d", pr.Repo, pr.Number), false)
							}
						}()
					},
				},
				components.ActionMenuItem{
					Label: "Merge", Key: "m",
					Action: func() {
						go func() {
							if err := a.client.Merge(a.ctx, pr.Repo, pr.Number); err != nil {
								a.flash.Show("Merge failed: "+err.Error(), true)
							} else {
								a.flash.Show(fmt.Sprintf("Merged %s#%d", pr.Repo, pr.Number), false)
							}
						}()
					},
				},
				components.ActionMenuItem{
					Label: "View diff", Key: "v",
					Action: func() {
						go func() {
							diff, err := a.client.GetPullDiff(a.ctx, pr.Repo, pr.Number)
							if err != nil {
								a.flash.Show("Diff fetch failed: "+err.Error(), true)
								return
							}
							a.logPane.SetContent(
								fmt.Sprintf("%s#%d", pr.Repo, pr.Number),
								diff, false,
							)
							a.logPane.CycleMode()
						}()
					},
				},
				components.ActionMenuItem{
					Label: "Open in browser", Key: "o",
					Action: func() { go openBrowser(pr.HTMLURL) },
				},
				components.ActionMenuItem{
					Label: "Dismiss", Key: "x",
					Action: func() {
						dismissKey := fmt.Sprintf("%s:%d", pr.Repo, pr.Number)
						a.db.AddDismissed(pr.Repo, pr.Number)
						a.dismissed[dismissKey] = true
						a.rebuildScreenData()
					},
				},
			)
		}
	}

	if len(items) > 0 {
		a.actionMenu.Show(items)
	}
```

**Step 3: Intercept keys when ActionMenu is active**

In `Update()`, before the ConfirmBar intercept, add ActionMenu intercept:

```go
case tea.KeyPressMsg:
	if a.actionMenu.Active {
		handled := a.actionMenu.HandleKey(msg.String())
		if handled {
			return a, nil
		}
	}
	if a.confirmBar.Active {
		// ... existing ...
	}
	return a.handleKey(msg)
```

**Step 4: Render ActionMenu overlay in View**

In `View()`, render ActionMenu as a popup overlay when active:

```go
if a.actionMenu.Active {
	// Overlay action menu on top of content
	menuRendered := a.actionMenu.Render()
	// Position in center-right of content area
	// Simple approach: append to content
	content = content + "\n" + menuRendered
}
```

**Step 5: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 6: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire ActionMenu — contextual actions popup on Enter key"
```

---

## Phase 3: Agent Integration

These tasks wire the agent dispatch, scheduling, and auto-fix infrastructure. They depend on Phase 2 (specifically Flash/ConfirmBar from Task 6). Tasks within this phase are sequential (each builds on the previous).

---

### Task 11: Wire Dispatcher to App

Instantiate the Dispatcher in App, wire the `D` key, show running agents on dashboard.

**Files:**
- Modify: `internal/app/app.go` (add dispatcher field, instantiate in NewApp, wire D key, update dashboard with agent status, cleanup on quit)

**Step 1: Add dispatcher field**

In App struct:

```go
dispatcher *agents.Dispatcher
```

**Step 2: Instantiate in NewApp**

In `NewApp()`, after config and DB are set up:

```go
a.dispatcher = agents.NewDispatcher(
	cfg.Agents.MaxConcurrent,
	cfg.Agents.MaxLifetime,
	cfg.Agents.CaptureOutput,
)
```

If `MaxConcurrent` is 0, default to 2. If `MaxLifetime` is 0, default to 1800.

**Step 3: Wire D key in handleDashboardKey**

```go
case key.Matches(msg, ui.Keys.Dispatch):
	if a.dashboard.Focus == screens.FocusPipeline {
		if run := a.dashboard.Pipeline.SelectedRun(); run != nil && run.Conclusion == "failure" {
			prompt := fmt.Sprintf("Fix the failing CI in %s. The workflow %s failed.", run.Repo, run.Name)
			a.confirmBar.Show(
				fmt.Sprintf("Dispatch fix agent for %s?", run.Name),
				func() {
					go func() {
						id, err := a.dispatcher.Dispatch(run.Repo, prompt)
						if err != nil {
							a.flash.Show("Dispatch failed: "+err.Error(), true)
						} else {
							a.flash.Show(fmt.Sprintf("Agent dispatched: %s", id), false)
						}
					}()
				},
				func() {},
			)
		}
	}
```

**Step 4: Feed dispatched agents to dashboard**

In `rebuildScreenData()`, add after the existing agent profiles section:

```go
// Dashboard — dispatched agents
if a.dispatcher != nil {
	a.dashboard.Dispatched = a.dispatcher.AllAgents()
}
```

**Step 5: Cleanup dispatcher on quit**

In the quit handling (handleKey), add dispatcher shutdown:

```go
case key.Matches(msg, ui.Keys.Quit):
	a.quitting = true
	if a.dispatcher != nil {
		a.dispatcher.Shutdown()
	}
	a.poller.Stop()
	a.cancel()
	return a, tea.Quit
```

**Step 6: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 7: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire Dispatcher — dispatch fix agents (D key), show on dashboard, cleanup on quit"
```

---

### Task 12: Wire Scheduler and Background Agent Loop

Instantiate the Scheduler, run a background loop that checks for due tasks and dispatches them.

**Files:**
- Modify: `internal/app/app.go` (add scheduler field, background tick message, handle tick)

**Step 1: Add scheduler field and tick message**

In App struct:

```go
scheduler *agents.Scheduler
```

Add a new message type:

```go
type agentTickMsg struct{}
```

Add a tick command:

```go
func agentTick() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return agentTickMsg{}
	})
}
```

**Step 2: Instantiate scheduler in NewApp**

```go
if len(cfg.Agents.Scheduled) > 0 {
	a.scheduler = agents.NewScheduler(cfg.Agents.Scheduled)
}
```

**Step 3: Start tick in Init**

In `Init()`, return both the poll wait and the agent tick:

```go
func (a App) Init() tea.Cmd {
	a.poller.Start(a.ctx)
	cmds := []tea.Cmd{waitForPoll(a.resultCh)}
	if a.scheduler != nil {
		cmds = append(cmds, agentTick())
	}
	return tea.Batch(cmds...)
}
```

**Step 4: Handle tick in Update**

Add to `Update()`:

```go
case agentTickMsg:
	// Check scheduled tasks
	if a.scheduler != nil && a.dispatcher != nil {
		for _, task := range a.scheduler.DueTasks(time.Now()) {
			repo := task.Config.Repo
			prompt := task.Config.Prompt
			if prompt == "" {
				// Workflow dispatch instead of local agent
				go func(r, wf string) {
					if err := a.client.DispatchWorkflow(a.ctx, r, wf); err != nil {
						slog.Error("scheduled dispatch failed", "name", task.Config.Name, "err", err)
					}
				}(repo, task.Config.Workflow)
			} else {
				go func(r, p string) {
					if _, err := a.dispatcher.Dispatch(r, p); err != nil {
						slog.Error("scheduled agent dispatch failed", "name", task.Config.Name, "err", err)
					}
				}(repo, prompt)
			}
		}
	}

	// Check agent lifetimes
	if a.dispatcher != nil {
		a.dispatcher.CheckAll()
		a.dashboard.Dispatched = a.dispatcher.AllAgents()
	}

	return a, agentTick()
```

**Step 5: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 6: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire Scheduler — cron-based agent dispatch with 10s background tick"
```

---

### Task 13: Wire AutoFixTracker

Instantiate AutoFixTracker, evaluate new failures in poll results, and dispatch fix agents automatically.

**Files:**
- Modify: `internal/app/app.go` (add autoFix field, instantiate, evaluate on poll)

**Step 1: Add autoFix field**

In App struct:

```go
autoFix *agents.AutoFixTracker
```

**Step 2: Instantiate in NewApp**

```go
a.autoFix = agents.NewAutoFixTracker(cfg.Agents.MaxConcurrent)
for _, repo := range cfg.Repos {
	for groupName, group := range repo.Groups {
		if group.AutoFix {
			cooldown := group.AutoFixCooldown
			if cooldown == 0 {
				cooldown = 300
			}
			a.autoFix.SetCooldown(repo.Repo, groupName, cooldown)
		}
	}
}
```

**Step 3: Evaluate on poll result**

In `handlePollResult()`, after persisting runs to DB and before `rebuildScreenData()`, add:

```go
// Auto-fix evaluation
if a.autoFix != nil && a.dispatcher != nil {
	for _, run := range msg.Result.Runs {
		if run.Conclusion != "failure" {
			continue
		}
		// Check if this workflow has auto_fix enabled
		repoConfig := a.findRepoConfig(run.Repo)
		if repoConfig == nil {
			continue
		}
		autoFixEnabled := false
		for _, group := range repoConfig.Groups {
			for _, wf := range group.Workflows {
				if wf == run.WorkflowFile && group.AutoFix {
					autoFixEnabled = true
				}
			}
		}
		if !autoFixEnabled {
			continue
		}

		// Get failed jobs
		var failedJobs []models.Job
		for _, j := range run.Jobs {
			if j.Conclusion == "failure" {
				failedJobs = append(failedJobs, j)
			}
		}

		decision := a.autoFix.Evaluate(
			run.Repo, run.WorkflowFile, failedJobs,
			len(a.dispatcher.RunningAgents()),
			a.db.IsKnownFailure,
		)
		if decision.ShouldDispatch {
			go func(repo, prompt string) {
				id, err := a.dispatcher.Dispatch(repo, prompt)
				if err != nil {
					slog.Error("auto-fix dispatch failed", "repo", repo, "err", err)
				} else {
					slog.Info("auto-fix dispatched", "repo", repo, "agent", id)
					a.flash.Show("Auto-fix agent dispatched", false)
				}
			}(decision.Repo, decision.Prompt)
		}
	}
}
```

Add a helper method to find repo config:

```go
func (a App) findRepoConfig(repo string) *config.RepoConfig {
	for i := range a.config.Repos {
		if a.config.Repos[i].Repo == repo {
			return &a.config.Repos[i]
		}
	}
	return nil
}
```

**Step 4: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 5: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire AutoFixTracker — auto-dispatch agents on CI failure with cooldown"
```

---

### Task 14: Wire CatchupOverlay

Track idle time and show the catchup overlay when user returns after idle period.

**Files:**
- Modify: `internal/app/app.go` (add lastInput/catchup fields, track input, trigger overlay)

**Step 1: Add fields**

In App struct:

```go
catchup       components.CatchupOverlay
lastInput     time.Time
preIdleRuns   int
preIdlePulls  int
preIdleTasks  int
```

**Step 2: Initialize lastInput**

In `NewApp()`:

```go
a.lastInput = time.Now()
```

**Step 3: Track last input and snapshot counts**

In `handleKey()`, at the top before the switch:

```go
now := time.Now()

// Check if returning from idle
if a.config.Catchup.Enabled && !a.catchup.Visible {
	threshold := time.Duration(a.config.Catchup.IdleThreshold) * time.Second
	if threshold == 0 {
		threshold = 15 * time.Minute
	}
	if now.Sub(a.lastInput) > threshold {
		// Count changes since idle
		totalRuns := 0
		totalPulls := 0
		for _, runs := range a.allRuns {
			totalRuns += len(runs)
		}
		for _, pulls := range a.allPulls {
			totalPulls += len(pulls)
		}
		newRuns := totalRuns - a.preIdleRuns
		changedPRs := totalPulls - a.preIdlePulls
		if newRuns > 0 || changedPRs > 0 {
			a.catchup.Show(newRuns, 0, changedPRs)
			a.lastInput = now
			return a, nil
		}
	}
}

a.lastInput = now
```

**Step 4: Dismiss overlay on Escape**

In `handleKey()`, in the Escape handling:

```go
case key.Matches(msg, ui.Keys.Escape):
	if a.catchup.Visible {
		a.catchup.Dismiss()
		return a, nil
	}
	// ... existing escape handling ...
```

**Step 5: Snapshot counts before idle**

In `handlePollResult()`, after rebuilding screen data, snapshot the counts:

```go
// Snapshot for catchup detection
totalRuns := 0
for _, runs := range a.allRuns {
	totalRuns += len(runs)
}
totalPulls := 0
for _, pulls := range a.allPulls {
	totalPulls += len(pulls)
}
// Only update snapshot if user is active (not idle)
if time.Since(a.lastInput) < 60*time.Second {
	a.preIdleRuns = totalRuns
	a.preIdlePulls = totalPulls
}
```

**Step 6: Render catchup in View**

In `View()`, when catchup is visible, render it as an overlay:

```go
if a.catchup.Visible {
	content = a.catchup.Render(a.width, contentHeight)
}
```

**Step 7: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 8: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: wire CatchupOverlay — show summary when returning from idle"
```

---

## Phase 4: Polish

These tasks improve UX. All are independent and can run in parallel.

---

### Task 15: Minimum Terminal Size Check

Show a helpful message when terminal is too small instead of rendering garbage.

**Files:**
- Modify: `internal/app/app.go:440-514` (View — add size check at top)

**Step 1: Add size check in View**

At the top of `View()`, after the quitting check:

```go
if a.width < 60 || a.height < 10 {
	msg := fmt.Sprintf("Terminal too small: %d×%d\nMinimum: 60×10\nPlease resize.", a.width, a.height)
	v := tea.NewView(msg)
	v.AltScreen = true
	return v
}
```

**Step 2: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 3: Commit**

```bash
git add internal/app/app.go
git commit -m "fix: show helpful message when terminal is too small (<60x10)"
```

---

### Task 16: Conditional Agent Roster

Hide the Agent Roster panel on dashboard when no agent workflows are configured.

**Files:**
- Modify: `internal/app/app.go` (add hasAgentWorkflows flag)
- Modify: `internal/ui/screens/dashboard.go` (skip roster panel when no agents)

**Step 1: Add flag to DashboardModel**

In `internal/ui/screens/dashboard.go`, add to DashboardModel struct:

```go
ShowRoster bool
```

**Step 2: Set flag in App**

In `internal/app/app.go`, in `NewApp()`, after building `agentWorkflows`:

```go
hasAgents := false
for _, wfs := range a.agentWorkflows {
	if len(wfs) > 0 {
		hasAgents = true
		break
	}
}
a.dashboard.ShowRoster = hasAgents
```

**Step 3: Modify dashboard Render to skip roster**

In `internal/ui/screens/dashboard.go`, in `Render()`, change the three-panel layout to conditionally show two panels:

```go
if d.ShowRoster {
	// Three-panel layout (existing)
	// pipeline | review | roster
} else {
	// Two-panel layout
	// pipeline (60%) | review (40%)
}
```

Adjust `CycleFocus()` to skip `FocusRoster` when `!ShowRoster`:

```go
func (d *DashboardModel) CycleFocus() {
	if d.ShowRoster {
		d.Focus = (d.Focus + 1) % 3
	} else {
		if d.Focus == FocusPipeline {
			d.Focus = FocusReview
		} else {
			d.Focus = FocusPipeline
		}
	}
}
```

**Step 4: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS (update existing dashboard tests if they assert on 3-panel layout).

**Step 5: Commit**

```bash
git add internal/app/app.go internal/ui/screens/dashboard.go
git commit -m "feat: hide Agent Roster panel when no agent workflows configured"
```

---

### Task 17: DB Error Visibility

Surface DB persistence errors in the status bar instead of silently logging them.

**Files:**
- Modify: `internal/app/app.go` (add dbErrorCount, show warning)

**Step 1: Add error counter to App**

In App struct:

```go
dbErrors int
```

**Step 2: Count and surface errors**

In `handlePollResult()`, replace the silent slog.Error calls:

```go
dbFailed := false
for _, run := range msg.Result.Runs {
	if err := a.db.UpsertRun(run); err != nil {
		slog.Error("upsert run", "err", err)
		dbFailed = true
	}
}
for _, pr := range msg.Result.PullRequests {
	if err := a.db.UpsertPull(pr); err != nil {
		slog.Error("upsert pull", "err", err)
		dbFailed = true
	}
}
if dbFailed {
	a.dbErrors++
	if a.dbErrors >= 3 {
		a.flash.Show("DB writes failing — data not persisted", true)
	}
} else {
	a.dbErrors = 0
}
```

**Step 3: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "fix: surface DB persistence errors in status bar after 3 consecutive failures"
```

---

### Task 18: Improved Agent Process Cleanup

Add SIGTERM → timeout → SIGKILL escalation to Dispatcher shutdown.

**Files:**
- Modify: `internal/agents/dispatch.go:189-198` (Shutdown)
- Add test to `internal/agents/agents_test.go`

**Step 1: Improve Shutdown with timeout escalation**

Replace the existing `Shutdown()` in `internal/agents/dispatch.go`:

```go
func (d *Dispatcher) Shutdown() {
	d.mu.Lock()
	var running []*DispatchedAgent
	for _, agent := range d.agents {
		if agent.Status == "running" {
			running = append(running, agent)
		}
	}
	d.mu.Unlock()

	// Send SIGTERM to all running agents
	for _, agent := range running {
		if agent.cmd.ProcessState == nil {
			slog.Info("terminating agent", "id", agent.ID, "pid", agent.PID)
			syscall.Kill(-agent.PID, syscall.SIGTERM)
		}
	}

	// Wait up to 5 seconds for graceful exit
	deadline := time.After(5 * time.Second)
	for _, agent := range running {
		if agent.cmd.ProcessState != nil {
			continue
		}
		done := make(chan struct{})
		go func(a *DispatchedAgent) {
			a.cmd.Wait()
			close(done)
		}(agent)

		select {
		case <-done:
			d.mu.Lock()
			agent.Status = "killed"
			d.mu.Unlock()
		case <-deadline:
			slog.Warn("agent did not exit after SIGTERM, sending SIGKILL", "id", agent.ID)
			syscall.Kill(-agent.PID, syscall.SIGKILL)
			d.mu.Lock()
			agent.Status = "killed"
			d.mu.Unlock()
		}
	}
}
```

**Step 2: Run all tests**

Run: `go test ./... -count=1`
Expected: All tests PASS.

**Step 3: Commit**

```bash
git add internal/agents/dispatch.go
git commit -m "fix: graceful agent shutdown — SIGTERM then SIGKILL after 5s timeout"
```

---

## Phase 5: Test Coverage

These tasks add tests for critical paths that currently have zero coverage.

---

### Task 19: App Integration Tests

Test the core App flow: poll result → screen data update → action dispatch.

**Files:**
- Create: `internal/app/app_test.go`

**Step 1: Write App integration tests**

Create `internal/app/app_test.go` with tests covering:
- `NewApp` creates valid App with all fields initialized
- `handlePollResult` updates allRuns/allPulls and rebuilds screen data
- `handlePollResult` with AuthError shows auth message
- `handlePollResult` with RateLimitError shows rate limit message
- `handleKey` screen switching (1/2/3/4)
- `handleDashboardKey` Tab cycles focus
- `handleDashboardKey` j/k navigates selector
- View() returns valid tea.View with AltScreen

Use in-memory SQLite (`db.OpenMemory()`) and a test HTTP server for the GitHub client. The poller can be nil for unit tests of handlePollResult (pass messages directly).

```go
package app

import (
	"testing"
	"time"

	"github.com/bruce-kelly/cimon/internal/config"
	"github.com/bruce-kelly/cimon/internal/db"
	ghclient "github.com/bruce-kelly/cimon/internal/github"
	"github.com/bruce-kelly/cimon/internal/models"
	"github.com/stretchr/testify/assert"
)

func testApp(t *testing.T) App {
	t.Helper()
	cfg := &config.CimonConfig{
		Repos: []config.RepoConfig{
			{Repo: "owner/repo", Branch: "main"},
		},
		Polling:     config.PollingConfig{Idle: 30, Active: 5, Cooldown: 3},
		ReviewQueue: config.ReviewQueueConfig{Escalation: config.EscalationConfig{Amber: 24, Red: 48}},
	}
	database, err := db.OpenMemory()
	assert.NoError(t, err)
	client := ghclient.NewTestClient("test-token", "http://localhost:1")
	return NewApp(cfg, client, database)
}

func TestNewApp_InitializesFields(t *testing.T) {
	a := testApp(t)
	assert.NotNil(t, a.config)
	assert.NotNil(t, a.client)
	assert.NotNil(t, a.db)
	assert.NotNil(t, a.allRuns)
	assert.NotNil(t, a.allPulls)
	assert.False(t, a.quitting)
}

func TestHandlePollResult_UpdatesData(t *testing.T) {
	a := testApp(t)
	result := pollResultMsg{
		Result: models.PollResult{
			Repo: "owner/repo",
			Runs: []models.WorkflowRun{
				{ID: 1, Name: "CI", Status: "completed", Conclusion: "success",
					Repo: "owner/repo", CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
			PullRequests: []models.PullRequest{
				{Number: 42, Title: "Fix bug", Repo: "owner/repo", State: "open",
					CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
			RateLimitRemaining: 4999,
		},
	}

	model, _ := a.handlePollResult(result)
	updated := model.(App)
	assert.Len(t, updated.allRuns["owner/repo"], 1)
	assert.Len(t, updated.allPulls["owner/repo"], 1)
	assert.Equal(t, 4999, updated.rateLimit)
}
```

**Step 2: Run tests**

Run: `go test ./internal/app/ -v -count=1`
Expected: All tests PASS.

**Step 3: Commit**

```bash
git add internal/app/app_test.go
git commit -m "test: add App integration tests — NewApp, handlePollResult, key handling"
```

---

### Task 20: Poller Integration Tests

Test the poller loop with a real HTTP server.

**Files:**
- Modify: `internal/polling/polling_test.go` (add integration tests)

**Step 1: Write poller integration test**

```go
func TestPoller_PollsAndSendsResults(t *testing.T) {
	// Create a test HTTP server that returns workflow runs
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/actions/runs"):
			json.NewEncoder(w).Encode(map[string]any{
				"workflow_runs": []map[string]any{
					{"id": 1, "name": "CI", "status": "completed", "conclusion": "success",
						"head_branch": "main", "created_at": time.Now().Format(time.RFC3339),
						"updated_at": time.Now().Format(time.RFC3339), "html_url": "https://example.com"},
				},
			})
		case strings.Contains(r.URL.Path, "/pulls"):
			json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	cfg := &config.CimonConfig{
		Repos: []config.RepoConfig{
			{Repo: "owner/repo", Branch: "main",
				Groups: map[string]config.GroupConfig{
					"ci": {Workflows: []string{"ci.yml"}},
				}},
		},
		Polling: config.PollingConfig{Idle: 1, Active: 1, Cooldown: 1},
	}

	resultCh := make(chan models.PollResult, 1)
	client := github.NewTestClient("test-token", srv.URL)
	p := New(client, cfg, resultCh)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p.Start(ctx)

	// Should receive at least one result
	select {
	case result := <-resultCh:
		assert.Equal(t, "owner/repo", result.Repo)
		assert.GreaterOrEqual(t, len(result.Runs), 0)
	case <-ctx.Done():
		t.Fatal("timed out waiting for poll result")
	}

	p.Stop()
}
```

**Step 2: Run tests**

Run: `go test ./internal/polling/ -v -count=1`
Expected: All tests PASS.

**Step 3: Commit**

```bash
git add internal/polling/polling_test.go
git commit -m "test: add poller integration test with httptest server"
```

---

## Task Dependency Graph

```
Phase 1 (all parallel):
  Task 1: ETag Cache ──┐
  Task 2: Channel Fix ─┤
  Task 3: HTTP Errors ─┼── Phase 2 begins after all Phase 1 complete
  Task 4: SQLite Safety┤
  Task 5: Data Fixes ──┘

Phase 2:
  Task 6: Flash/ConfirmBar ──┐ (do first)
  Task 7: Pipeline Actions ──┤
  Task 8: Review Actions ────┼── All depend on Task 6
  Task 9: LogPane ───────────┤
  Task 10: ActionMenu ───────┘

Phase 3 (sequential):
  Task 11: Dispatcher ──→ Task 12: Scheduler ──→ Task 13: AutoFix ──→ Task 14: Catchup

Phase 4 (all parallel, after Phase 2):
  Task 15: Terminal Size
  Task 16: Conditional Roster
  Task 17: DB Error Visibility
  Task 18: Agent Cleanup

Phase 5 (after Phase 3):
  Task 19: App Tests
  Task 20: Poller Tests
```

## Parallelization Strategy for Agents

**Wave 1** (5 agents in parallel): Tasks 1, 2, 3, 4, 5
**Wave 2** (1 agent): Task 6
**Wave 3** (4 agents in parallel): Tasks 7, 8, 9, 10
**Wave 4** (4 agents in parallel): Tasks 11, 15, 16, 17
**Wave 5** (sequential on main): Tasks 12, 13, 14, 18
**Wave 6** (2 agents in parallel): Tasks 19, 20

Total: 20 tasks across 6 waves. With agent parallelization, effective serial path is ~8 tasks.
