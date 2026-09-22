package notifier

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	mail "github.com/wneessen/go-mail"
)

func TestEmailAlertAndAllClear(t *testing.T) {
	var messages []*mail.Msg
	var deliveries []emailDelivery
	email := &Email{
		From:              "nanny@example.com",
		To:                []string{"one@example.com", "two@example.com"},
		Subject:           "Alert: %s",
		SubjectAllClear:   "Clear: %s",
		Body:              "<strong>%s</strong>",
		Server:            "smtp.example.com",
		Port:              465,
		User:              "user",
		Password:          "password",
		AllowInsecureAuth: true,
		send: func(ctx context.Context, message *mail.Msg, delivery emailDelivery) error {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 10*time.Second {
				t.Fatal("send context does not have the expected 10-second timeout")
			}
			messages = append(messages, message)
			deliveries = append(deliveries, delivery)
			return nil
		},
	}
	msg := Message{
		Nanny:      "Nanny",
		Program:    "cron",
		NextSignal: time.Minute,
		Meta:       map[string]string{"environment": "test"},
	}

	if err := email.Notify(msg); err != nil {
		t.Fatal(err)
	}
	if err := email.NotifyAllClear(msg); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || len(deliveries) != 2 {
		t.Fatalf("sent %d messages with %d deliveries", len(messages), len(deliveries))
	}
	if got := messages[0].GetGenHeader(mail.HeaderSubject); len(got) != 1 || got[0] != "Alert: cron" {
		t.Fatalf("alert subject = %#v", got)
	}
	if got := messages[1].GetGenHeader(mail.HeaderSubject); len(got) != 1 || got[0] != "Clear: cron" {
		t.Fatalf("all-clear subject = %#v", got)
	}
	if got := messages[0].GetFromString(); len(got) != 1 || !strings.Contains(got[0], "nanny@example.com") {
		t.Fatalf("from = %#v", got)
	}
	if got := messages[0].GetToString(); len(got) != 2 {
		t.Fatalf("to = %#v", got)
	}

	for i, wantText := range []string{msg.Format(), msg.FormatAllClear()} {
		var body bytes.Buffer
		if _, err := messages[i].WriteTo(&body); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body.String(), wantText) || !strings.Contains(body.String(), "environment") {
			t.Fatalf("message body = %q", body.String())
		}
	}

	delivery := deliveries[0]
	if delivery.server != email.Server || delivery.port != email.Port || delivery.user != email.User || delivery.password != email.Password {
		t.Fatalf("delivery = %+v", delivery)
	}
	if !delivery.implicitTLS || !delivery.allowInsecureAuth {
		t.Fatalf("delivery security = %+v", delivery)
	}
}

func TestEmailUsesOpportunisticSTARTTLS(t *testing.T) {
	email := &Email{
		From:    "nanny@example.com",
		To:      []string{"one@example.com"},
		Subject: "%s",
		Body:    "%s",
		Port:    587,
		send: func(_ context.Context, _ *mail.Msg, delivery emailDelivery) error {
			if delivery.implicitTLS {
				t.Fatal("port 587 selected implicit TLS")
			}
			return nil
		},
	}
	if err := email.Notify(Message{}); err != nil {
		t.Fatal(err)
	}
}

func TestEmailAuthenticationSelection(t *testing.T) {
	secure := smtpAuthTypes(false)
	if slices.Contains(secure, mail.SMTPAuthPlainNoEnc) || slices.Contains(secure, mail.SMTPAuthLoginNoEnc) {
		t.Fatalf("secure auth types contain plaintext fallback: %#v", secure)
	}
	insecure := smtpAuthTypes(true)
	if !slices.Contains(insecure, mail.SMTPAuthPlainNoEnc) || !slices.Contains(insecure, mail.SMTPAuthLoginNoEnc) {
		t.Fatalf("insecure auth types = %#v", insecure)
	}
}

func TestEmailClientValidation(t *testing.T) {
	if _, err := newMailClient(emailDelivery{port: 587}); err == nil {
		t.Fatal("newMailClient() error = nil, want missing server error")
	}
}

func TestEmailSendFailure(t *testing.T) {
	wantErr := errors.New("send failed")
	email := &Email{
		From:    "nanny@example.com",
		To:      []string{"one@example.com"},
		Subject: "%s",
		Body:    "%s",
		send: func(context.Context, *mail.Msg, emailDelivery) error {
			return wantErr
		},
	}
	if err := email.Notify(Message{}); !errors.Is(err, wantErr) {
		t.Fatalf("Notify() error = %v, want %v", err, wantErr)
	}
}
