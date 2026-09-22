package notifier

import (
	"errors"
	"testing"
	"time"

	twilioclient "github.com/twilio/twilio-go/client"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

func TestNewTwilio(t *testing.T) {
	n, ok := NewTwilio("AC123", "token", "AP123", "+12025550100", "+12025550101").(*twilio)
	if !ok {
		t.Fatal("NewTwilio() did not return the Twilio notifier")
	}
	if n.from != "+12025550100" || n.to != "+12025550101" || n.appSid != "AP123" {
		t.Fatalf("unexpected Twilio configuration: %#v", n)
	}
	if n.createMessage == nil {
		t.Fatal("NewTwilio() did not initialize the message sender")
	}
}

func TestTwilioCredentialsDoNotUseAmbientEnvironment(t *testing.T) {
	t.Setenv("TWILIO_ACCOUNT_SID", "ambient-account")
	t.Setenv("TWILIO_AUTH_TOKEN", "ambient-token")

	restClient := newTwilioRestClient("", "")
	baseClient, ok := restClient.Client.(*twilioclient.Client)
	if !ok {
		t.Fatalf("unexpected Twilio base client type %T", restClient.Client)
	}
	if baseClient.Username != "" || baseClient.Password != "" || baseClient.AccountSid() != "" {
		t.Fatalf("Twilio client used ambient credentials: %#v", baseClient.Credentials)
	}
}

func TestTwilioNotifications(t *testing.T) {
	message := Message{
		Nanny:      "Home",
		Program:    "backup",
		NextSignal: 5 * time.Minute,
	}
	tests := []struct {
		name       string
		appSid     string
		notify     func(*twilio, Message) error
		wantBody   string
		wantAppSid bool
	}{
		{
			name:       "alert with application SID",
			appSid:     "AP123",
			notify:     (*twilio).Notify,
			wantBody:   message.Format(),
			wantAppSid: true,
		},
		{
			name:     "all clear without application SID",
			notify:   (*twilio).NotifyAllClear,
			wantBody: message.FormatAllClear(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got *openapi.CreateMessageParams
			status := "queued"
			n := &twilio{
				from:   "+12025550100",
				to:     "+12025550101",
				appSid: test.appSid,
				createMessage: func(params *openapi.CreateMessageParams) (*openapi.ApiV2010Message, error) {
					got = params
					return &openapi.ApiV2010Message{Status: &status}, nil
				},
			}

			if err := test.notify(n, message); err != nil {
				t.Fatal(err)
			}
			if got == nil || got.From == nil || *got.From != n.from || got.To == nil || *got.To != n.to {
				t.Fatalf("unexpected sender or recipient: %#v", got)
			}
			if got.Body == nil || *got.Body != test.wantBody {
				t.Fatalf("body = %v, want %q", got.Body, test.wantBody)
			}
			if (got.ApplicationSid != nil) != test.wantAppSid {
				t.Fatalf("ApplicationSid = %v, want present %v", got.ApplicationSid, test.wantAppSid)
			}
			if test.wantAppSid && *got.ApplicationSid != test.appSid {
				t.Fatalf("ApplicationSid = %q, want %q", *got.ApplicationSid, test.appSid)
			}
		})
	}
}

func TestTwilioSendErrors(t *testing.T) {
	sdkError := errors.New("Twilio unavailable")
	tests := []struct {
		name      string
		result    *openapi.ApiV2010Message
		err       error
		wantError bool
	}{
		{name: "SDK error", err: sdkError, wantError: true},
		{name: "nil response", wantError: true},
		{name: "failed status", result: messageWithStatus("failed"), wantError: true},
		{name: "undelivered status", result: messageWithStatus("undelivered"), wantError: true},
		{name: "missing status", result: &openapi.ApiV2010Message{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			n := &twilio{
				createMessage: func(*openapi.CreateMessageParams) (*openapi.ApiV2010Message, error) {
					return test.result, test.err
				},
			}
			err := n.Notify(Message{})
			if (err != nil) != test.wantError {
				t.Fatalf("Notify() error = %v, wantError %v", err, test.wantError)
			}
			if test.err != nil && !errors.Is(err, test.err) {
				t.Fatalf("Notify() error = %v, want wrapped %v", err, test.err)
			}
		})
	}
}

func messageWithStatus(status string) *openapi.ApiV2010Message {
	return &openapi.ApiV2010Message{Status: &status}
}
