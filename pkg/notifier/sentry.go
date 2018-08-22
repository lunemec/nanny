package notifier

import (
	"github.com/getsentry/raven-go"
	"github.com/pkg/errors"
)

type sentry struct {
	cli *raven.Client
}

// NewSentry creates sentry notifier from supplied DSN.
func NewSentry(dsn string) (Notifier, error) {
	cli, err := raven.New(dsn)
	if err != nil {
		return nil, errors.Wrap(err, "unable to initialize sentry")
	}
	return &sentry{cli: cli}, nil
}

// Notify implements Notifier interface for sentry.
func (n *sentry) Notify(msg Message) error {
	// This may block since it is run in its own goroutine.
	n.cli.CaptureMessageAndWait(msg.Format(), msg.Meta)
	return nil
}

func (n *sentry) String() string {
	return "sentry"
}
