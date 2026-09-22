package notifier

import (
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
)

type sentryNotifier struct {
	capture func(string, map[string]string) *sentry.EventID
}

// NewSentry creates sentry notifier from supplied DSN.
func NewSentry(dsn string) (Notifier, error) {
	transport := sentry.NewHTTPSyncTransport()
	transport.Timeout = 10 * time.Second
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:                    dsn,
		Transport:              transport,
		DisableTelemetryBuffer: true,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to initialize sentry: %w", err)
	}

	return &sentryNotifier{
		capture: func(message string, tags map[string]string) *sentry.EventID {
			scope := sentry.NewScope()
			scope.SetTags(tags)
			return client.CaptureMessage(message, nil, scope)
		},
	}, nil
}

func (n *sentryNotifier) send(message string, tags map[string]string) error {
	if eventID := n.capture(message, tags); eventID == nil {
		return fmt.Errorf("unable to notify via sentry: event was not accepted")
	}
	return nil
}

// Notify implements Notifier interface for sentry.
func (n *sentryNotifier) Notify(msg Message) error {
	return n.send(msg.Format(), msg.Meta)
}

// NotifyAllClear implements Notifier interface for sentry.
func (n *sentryNotifier) NotifyAllClear(msg Message) error {
	return n.send(msg.FormatAllClear(), msg.Meta)
}

func (n *sentryNotifier) String() string {
	return "sentry"
}
