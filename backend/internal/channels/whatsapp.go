package channels

import (
	"context"
	"fmt"
	"strings"

	"github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

type WhatsAppClient interface {
	Send(ctx context.Context, to, message string) error
}

// NoopClient is used in tests and when Twilio is not configured.
type NoopClient struct{}

func (n *NoopClient) Send(ctx context.Context, to, message string) error {
	return nil
}

// TwilioClient sends WhatsApp messages via the Twilio API.
type TwilioClient struct {
	client *twilio.RestClient
	from   string
}

func NewTwilioClient(accountSID, authToken, from string) *TwilioClient {
	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: accountSID,
		Password: authToken,
	})
	return &TwilioClient{client: client, from: from}
}

func (c *TwilioClient) Send(ctx context.Context, to, message string) error {
	params := &openapi.CreateMessageParams{}
	if !strings.HasPrefix(to, "whatsapp:") {
		to = "whatsapp:" + to
	}
	params.SetTo(to)
	params.SetFrom(c.from)
	params.SetBody(message)

	_, err := c.client.Api.CreateMessage(params)
	if err != nil {
		return fmt.Errorf("twilio send to %s: %w", to, err)
	}
	return nil
}
