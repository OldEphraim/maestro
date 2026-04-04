package channels

import "context"

type WhatsAppClient interface {
	Send(ctx context.Context, to, message string) error
}

// NoopClient is used in tests and when Twilio is not configured.
type NoopClient struct{}

func (n *NoopClient) Send(ctx context.Context, to, message string) error {
	return nil
}
