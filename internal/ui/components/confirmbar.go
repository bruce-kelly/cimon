package components

// ConfirmBar shows a y/n confirmation prompt.
type ConfirmBar struct {
	Active    bool
	Message   string
	OnConfirm func()
	OnCancel  func()
}

func (c *ConfirmBar) Show(message string, onConfirm, onCancel func()) {
	c.Active = true
	c.Message = message
	c.OnConfirm = onConfirm
	c.OnCancel = onCancel
}

func (c *ConfirmBar) HandleKey(keyStr string) bool {
	if !c.Active {
		return false
	}
	switch keyStr {
	case "y":
		c.Active = false
		if c.OnConfirm != nil {
			c.OnConfirm()
		}
		return true
	case "n", "esc":
		c.Active = false
		if c.OnCancel != nil {
			c.OnCancel()
		}
		return true
	}
	return true // consume all keys while active
}

func (c *ConfirmBar) Render(width int) string {
	if !c.Active {
		return ""
	}
	return " " + c.Message + " [y/n/Esc]"
}
