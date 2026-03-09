package components

// Selector provides j/k cursor navigation for list components.
type Selector struct {
	index int
	count int
}

func (s *Selector) SetCount(n int) {
	s.count = n
	if n == 0 {
		s.index = 0
		return
	}
	if s.index >= n {
		s.index = n - 1
	}
}

func (s *Selector) Next() {
	if s.count == 0 {
		return
	}
	s.index = (s.index + 1) % s.count
}

func (s *Selector) Prev() {
	if s.count == 0 {
		return
	}
	s.index = (s.index - 1 + s.count) % s.count
}

func (s *Selector) Index() int { return s.index }
func (s *Selector) Count() int { return s.count }
