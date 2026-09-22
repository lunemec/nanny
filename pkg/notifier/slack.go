package notifier

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"github.com/slack-go/slack"
)

var slackHTTPClient = &http.Client{Timeout: 10 * time.Second}

type slackNotifier struct {
	webhookURL string
}

// NewSlack creates a new slack notifier sending to the supplied webhookURL.
func NewSlack(webhookURL string) (Notifier, error) {
	if webhookURL == "" {
		return nil, errors.New("Unable to initialize slack: webhookURL is empty")
	}
	parsedURL, err := url.Parse(webhookURL)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to initialze slack")
	}
	if parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, errors.New("Unable to initialize slack: webhookURL must be an absolute HTTP or HTTPS URL")
	}

	return &slackNotifier{webhookURL}, nil
}

// Notify implements Notifier interface for slack.
func (s *slackNotifier) Notify(msg Message) error {
	return s.send(msg.Format(), msg.Meta, "#FF0000")
}

// NotifyAllClear implements Notifier interface for slack.
func (s *slackNotifier) NotifyAllClear(msg Message) error {
	return s.send(msg.FormatAllClear(), msg.Meta, "#00FF00")
}

func (s *slackNotifier) send(text string, meta map[string]string, color string) error {
	attachment := slack.Attachment{
		Fallback: text,
		Text:     text,
		Color:    color,
	}
	for key, value := range meta {
		attachment.Fields = append(attachment.Fields, slack.AttachmentField{
			Title: key,
			Value: value,
		})
	}
	payload := &slack.WebhookMessage{
		Username:    "Nanny",
		IconEmoji:   ":baby_chick:",
		Attachments: []slack.Attachment{attachment},
	}
	if err := slack.PostWebhookCustomHTTPContext(
		context.Background(),
		s.webhookURL,
		slackHTTPClient,
		payload,
	); err != nil {
		return fmt.Errorf("unable to notify via Slack: %w", err)
	}
	return nil
}

func (s *slackNotifier) String() string {
	return "slack"
}
