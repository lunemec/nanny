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

// NewTwilio creates twilio sms sending notifier.
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
	if exc != nil {
		return errors.Errorf("error while sending SMS via twilio: %+v", exc)
	}
	// I know it does not make sense that resp would be nil, but since the type
	// is *gotwilio.SmsResponse we should check.
	if resp == nil {
		return errors.Errorf("*gotwilio.SmsResponse is nil, WUT?")
	}
	if resp.Status == "undelivered" || resp.Status == "failed" {
		return errors.Errorf("unable to send SMS via twilio: %+v", resp)
	}
	return nil
}

func (n *twilio) String() string {
	return "twilio"
}
