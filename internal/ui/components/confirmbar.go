package components

import tea "charm.land/bubbletea/v2"

// ConfirmBar shows a y/n confirmation prompt.
type ConfirmBar struct {
	Active    bool
	Message   string
	OnConfirm func() tea.Cmd
	OnCancel  func() tea.Cmd
}

func (c *ConfirmBar) Show(message string, onConfirm, onCancel func() tea.Cmd) {
	c.Active = true
	c.Message = message
	c.OnConfirm = onConfirm
	c.OnCancel = onCancel
}

func (c *ConfirmBar) HandleKey(keyStr string) (bool, tea.Cmd) {
	if !c.Active {
		return false, nil
	}
	switch keyStr {
	case "y":
		c.Active = false
		var cmd tea.Cmd
		if c.OnConfirm != nil {
			cmd = c.OnConfirm()
		}
		return true, cmd
	case "n", "esc":
		c.Active = false
		var cmd tea.Cmd
		if c.OnCancel != nil {
			cmd = c.OnCancel()
		}
		return true, cmd
	}
	return true, nil // consume all keys while active
}

func (c *ConfirmBar) Render(width int) string {
	if !c.Active {
		return ""
	}
	return " " + c.Message + " [y/n/Esc]"
}
