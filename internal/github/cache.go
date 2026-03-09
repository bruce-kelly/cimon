package github

import (
	"container/list"
	"sync"
)

// etagEntry holds a cached response with its ETag.
type etagEntry struct {
	url  string
	etag string
	body []byte
	elem *list.Element
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
