package channels

import (
	"errors"
	"net/http"
)

// ParseTwilioWebhook extracts the sender phone number and message body from
// a Twilio inbound webhook request (form-encoded POST).
// TODO: validate X-Twilio-Signature in production
func ParseTwilioWebhook(r *http.Request) (from, body string, err error) {
	if err := r.ParseForm(); err != nil {
		return "", "", err
	}
	from = r.FormValue("From")
	body = r.FormValue("Body")
	if from == "" {
		return "", "", errors.New("missing From field in Twilio webhook")
	}
	if body == "" {
		return "", "", errors.New("missing Body field in Twilio webhook")
	}
	return from, body, nil
}
