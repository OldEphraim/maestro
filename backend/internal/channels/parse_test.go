package channels_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/oldephraim/maestro/backend/internal/channels"
)

func TestParseTwilioWebhook(t *testing.T) {
	form := url.Values{
		"From":           {"whatsapp:+14405239475"},
		"Body":           {"Hello from WhatsApp"},
		"To":             {"whatsapp:+14155238886"},
		"AccountSid":     {"AC1234567890"},
		"MessageSid":     {"SM1234567890"},
		"NumMedia":       {"0"},
		"SmsMessageSid":  {"SM1234567890"},
		"SmsSid":         {"SM1234567890"},
		"SmsStatus":      {"received"},
	}

	req, _ := http.NewRequest("POST", "/api/webhooks/whatsapp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	from, body, err := channels.ParseTwilioWebhook(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if from != "whatsapp:+14405239475" {
		t.Errorf("expected from 'whatsapp:+14405239475', got %q", from)
	}
	if body != "Hello from WhatsApp" {
		t.Errorf("expected body 'Hello from WhatsApp', got %q", body)
	}
}

func TestParseTwilioWebhookMissingFrom(t *testing.T) {
	form := url.Values{
		"Body": {"Hello"},
	}
	req, _ := http.NewRequest("POST", "/api/webhooks/whatsapp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, _, err := channels.ParseTwilioWebhook(req)
	if err == nil {
		t.Fatal("expected error for missing From field")
	}
}

func TestParseTwilioWebhookMissingBody(t *testing.T) {
	form := url.Values{
		"From": {"whatsapp:+14405239475"},
	}
	req, _ := http.NewRequest("POST", "/api/webhooks/whatsapp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, _, err := channels.ParseTwilioWebhook(req)
	if err == nil {
		t.Fatal("expected error for missing Body field")
	}
}
