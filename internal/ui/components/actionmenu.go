package components

// ActionMenuItem represents a single action in the menu.
type ActionMenuItem struct {
	Label  string
	Key    string
	Action func()
}

// ActionMenu is a contextual popup menu.
type ActionMenu struct {
	Active   bool
	Items    []ActionMenuItem
	Selector Selector
}

func (m *ActionMenu) Show(items []ActionMenuItem) {
	m.Active = true
	m.Items = items
	m.Selector = Selector{}
	m.Selector.SetCount(len(items))
}

func (m *ActionMenu) Hide() {
	m.Active = false
	m.Items = nil
}

func (m *ActionMenu) HandleKey(keyStr string) bool {
	if !m.Active {
		return false
	}
	switch keyStr {
	case "j", "down":
		m.Selector.Next()
		return true
	case "k", "up":
		m.Selector.Prev()
		return true
	case "enter":
		idx := m.Selector.Index()
		if idx < len(m.Items) && m.Items[idx].Action != nil {
			m.Items[idx].Action()
		}
		m.Hide()
		return true
	case "esc", "q":
		m.Hide()
		return true
	}
	return true // consume while active
}

func (m *ActionMenu) Render() string {
	if !m.Active || len(m.Items) == 0 {
		return ""
	}
	var result string
	for i, item := range m.Items {
		prefix := "  "
		if i == m.Selector.Index() {
			prefix = "> "
		}
		result += prefix + item.Key + "  " + item.Label + "\n"
	}
	return result
}
