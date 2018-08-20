package notifier

import (
	"fmt"
	"net/url"

	"github.com/ashwanthkumar/slack-go-webhook"
	"github.com/pkg/errors"
)

type slackNotifier struct {
	webhookURL string
}

// NewSlack creates a new slack notifier sending to the supplied webhookURL.
func NewSlack(webhookURL string) (Notifier, error) {
	if webhookURL == "" {
		return nil, errors.New("Unable to initialize slack: webhookURL is empty")
	}
	if _, err := url.Parse(webhookURL); err != nil {
		return nil, errors.Wrap(err, "Unable to initialze slack")
	}

	return &slackNotifier{webhookURL}, nil
}

// Notify implements Notifier interface for slack.
func (s *slackNotifier) Notify(msg Message) error {
	msgText := msg.Format()
	hexRed := "#FF0000"
	attachment := slack.Attachment{
		Fallback: &msgText,
		Text:     &msgText,
		Color:    &hexRed,
	}
	if len(msg.Meta) != 0 {
		for key, value := range msg.Meta {
			attachment.AddField(slack.Field{Title: key, Value: value})
		}
	}
	payload := slack.Payload{
		Username:    "Nanny",
		IconEmoji:   ":baby_chick:",
		Attachments: []slack.Attachment{attachment},
	}
	errs := slack.Send(s.webhookURL, "", payload)
	if len(errs) > 0 {
		errStr := ""
		for _, err := range errs {
			errStr = fmt.Sprintf("%s, ", err)
		}
		return errors.New(errStr)
	}
	return nil
}

func (s *slackNotifier) String() string {
	return "slack"
}
