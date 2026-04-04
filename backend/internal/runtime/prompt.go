package runtime

import (
	"fmt"
	"strings"

	"github.com/oldephraim/maestro/backend/internal/agent"
)

func buildSystemPrompt(ag agent.AgentWithMemory) string {
	var sb strings.Builder
	sb.WriteString(ag.SystemPrompt)

	if len(ag.Memory) > 0 {
		sb.WriteString("\n\n## Memory\n")
		for k, v := range ag.Memory {
			fmt.Fprintf(&sb, "- %s: %s\n", k, v)
		}
	}

	return sb.String()
}

func buildFullPrompt(ag agent.AgentWithMemory, task string) string {
	sys := buildSystemPrompt(ag)
	return sys + "\n\nTask: " + task
}
