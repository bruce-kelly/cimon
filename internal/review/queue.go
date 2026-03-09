package review

import "sync"

// Queue holds the current set of review items with thread-safe access.
type Queue struct {
	items []ReviewItem
	mu    sync.RWMutex
}

func NewQueue() *Queue {
	return &Queue{}
}

func (q *Queue) Update(items []ReviewItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = items
}

func (q *Queue) Items() []ReviewItem {
	q.mu.RLock()
	defer q.mu.RUnlock()
	result := make([]ReviewItem, len(q.items))
	copy(result, q.items)
	return result
}

func (q *Queue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items)
}
