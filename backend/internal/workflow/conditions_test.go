package workflow

import "testing"

func TestEvaluateCondition(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		condition string
		want      bool
	}{
		{"always matches", "anything", "always", true},
		{"empty string matches", "anything", "", true},
		{"approved matches APPROVED", "The code looks good. APPROVED", "approved", true},
		{"approved case insensitive", "Approved by reviewer", "approved", true},
		{"approved no match", "The code needs work", "approved", false},
		{"rejected matches REJECTED", "REJECTED: missing error handling", "rejected", true},
		{"rejected case insensitive", "Rejected: no idempotency", "rejected", true},
		{"rejected no match", "The code is fine", "rejected", false},
		{"substring match", "error: timeout occurred", "timeout", true},
		{"substring no match", "everything is fine", "timeout", false},
		{"substring case sensitive", "TIMEOUT occurred", "timeout", false},
		{"always with whitespace", "anything", "  always  ", true},
		{"approved with whitespace", "APPROVED", "  Approved  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateCondition(tt.output, tt.condition)
			if got != tt.want {
				t.Errorf("evaluateCondition(%q, %q) = %v, want %v", tt.output, tt.condition, got, tt.want)
			}
		})
	}
}

func TestEvaluateConditionFirstMatch(t *testing.T) {
	// Output contains both APPROVED and REJECTED
	output := "REJECTED: missing idempotency. But overall APPROVED with caveats."

	// With priority-ordered edges: rejected first (p0), approved second (p1)
	edges := []WorkflowEdge{
		{Condition: "rejected", Priority: 0},
		{Condition: "approved", Priority: 1},
	}

	// First match should be "rejected"
	for _, edge := range edges {
		if evaluateCondition(output, edge.Condition) {
			if edge.Condition != "rejected" {
				t.Errorf("expected first match to be 'rejected', got %q", edge.Condition)
			}
			return
		}
	}
	t.Error("no condition matched")
}

func TestParseWhatsAppAction(t *testing.T) {
	tests := []struct {
		line    string
		wantTo  string
		wantMsg string
		wantOk  bool
	}{
		{"ACTION:WHATSAPP: +14155551234 | Hello there", "+14155551234", "Hello there", true},
		{"ACTION:WHATSAPP:+14155551234|Hello", "+14155551234", "Hello", true},
		{"Not an action", "", "", false},
		{"WHATSAPP: +14155551234 | Hello", "", "", false},
		{"ACTION:WHATSAPP: | Hello", "", "", false},
		{"ACTION:WHATSAPP: +14155551234 |", "", "", false},
	}

	for _, tt := range tests {
		to, msg, ok := parseWhatsAppAction(tt.line)
		if ok != tt.wantOk {
			t.Errorf("parseWhatsAppAction(%q) ok = %v, want %v", tt.line, ok, tt.wantOk)
			continue
		}
		if to != tt.wantTo {
			t.Errorf("parseWhatsAppAction(%q) to = %q, want %q", tt.line, to, tt.wantTo)
		}
		if msg != tt.wantMsg {
			t.Errorf("parseWhatsAppAction(%q) msg = %q, want %q", tt.line, msg, tt.wantMsg)
		}
	}
}
