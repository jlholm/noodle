package loop

import (
	"strings"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/shellx"
)

func orderWorktreeName(orderID string) string {
	return shellx.SanitizeToken(orderID, "order")
}

func renderBranchPattern(pattern, orderID, title string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		pattern = config.DefaultBranchPattern
	}
	todoID := shellx.SanitizeToken(orderID, "order")
	slug := shellx.SanitizeToken(title, "item")
	rendered := strings.ReplaceAll(pattern, "{todo_id}", todoID)
	return strings.ReplaceAll(rendered, "{slug}", slug)
}
