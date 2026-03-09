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
