package notifier

import (
	"errors"
	"fmt"

	twiliosdk "github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

type twilio struct {
	from string
	to   string

	appSid        string
	createMessage func(*openapi.CreateMessageParams) (*openapi.ApiV2010Message, error)
}

// NewTwilio creates twilio sms sending notifier.
func NewTwilio(accountSid, authToken, appSid, from, to string) Notifier {
	client := twiliosdk.NewRestClientWithParams(twiliosdk.ClientParams{
		Username:   accountSid,
		Password:   authToken,
		AccountSid: accountSid,
	})
	return &twilio{
		from:          from,
		to:            to,
		appSid:        appSid,
		createMessage: client.Api.CreateMessage,
	}
}

// Notify implements Notifier interface for twilio.
func (n *twilio) Notify(msg Message) error {
	return n.send(msg.Format())
}

// NotifyAllClear implements Notifier interface for twilio.
func (n *twilio) NotifyAllClear(msg Message) error {
	return n.send(msg.FormatAllClear())
}

func (n *twilio) send(body string) error {
	params := new(openapi.CreateMessageParams)
	params.SetFrom(n.from).SetTo(n.to).SetBody(body)
	if n.appSid != "" {
		params.SetApplicationSid(n.appSid)
	}

	resp, err := n.createMessage(params)
	if err != nil {
		return fmt.Errorf("unable to send SMS via Twilio: %w", err)
	}
	if resp == nil {
		return errors.New("unable to send SMS via Twilio: empty response")
	}
	if resp.Status != nil && (*resp.Status == "undelivered" || *resp.Status == "failed") {
		return fmt.Errorf("unable to send SMS via Twilio: status %s", *resp.Status)
	}
	return nil
}

func (n *twilio) String() string {
	return "twilio"
}
