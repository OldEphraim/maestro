package workflow

import "strings"

func parseWhatsAppAction(line string) (to, message string, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "ACTION:WHATSAPP:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(line, "ACTION:WHATSAPP:")
	parts := strings.SplitN(rest, "|", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	to = strings.TrimSpace(parts[0])
	message = strings.TrimSpace(parts[1])
	if to == "" || message == "" {
		return "", "", false
	}
	return to, message, true
}
