package notifier

import (
	"github.com/pkg/errors"
	"github.com/sfreiberg/gotwilio"
)

type twilio struct {
	from string
	to   string

	appSid string
	t      *gotwilio.Twilio
}

func NewTwilio(accountSid, authToken, appSid, from, to string) Notifier {
	return &twilio{
		t:    gotwilio.NewTwilioClient(accountSid, authToken),
		from: from,
		to:   to,
	}
}

func (n *twilio) Notify(msg Message) error {
	resp, exc, err := n.t.SendSMS(n.from, n.to, msg.Format(), "", n.appSid)
	if err != nil {
		return errors.Wrap(err, "unable to send SMS via twilio")
	}
	if exc.Status != 0 {
		return errors.Errorf("unable to send SMS via twilio: %+v", exc)
	}
	if resp.Status == "undelivered" || resp.Status == "failed" {
		return errors.Errorf("unable to send SMS via twilio: %+v", resp)
	}
	return nil
}
