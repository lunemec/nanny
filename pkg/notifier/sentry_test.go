package notifier

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

func TestNewSentryRejectsInvalidDSN(t *testing.T) {
	if _, err := NewSentry("://invalid"); err == nil {
		t.Fatal("NewSentry() error = nil, want invalid DSN error")
	}
}

func TestSentryAlertAndAllClear(t *testing.T) {
	eventID := sentry.EventID(strings.Repeat("a", 32))
	var messages []string
	var tags []map[string]string
	var levels []sentry.Level
	notifier := &sentryNotifier{
		capture: func(message string, metadata map[string]string, level sentry.Level) *sentry.EventID {
			messages = append(messages, message)
			tags = append(tags, metadata)
			levels = append(levels, level)
			return &eventID
		},
	}
	msg := Message{
		Nanny:      "Nanny",
		Program:    "cron",
		NextSignal: time.Minute,
		Meta:       map[string]string{"environment": "test"},
	}

	if err := notifier.Notify(msg); err != nil {
		t.Fatal(err)
	}
	if err := notifier.NotifyAllClear(msg); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0] != msg.Format() || messages[1] != msg.FormatAllClear() {
		t.Fatalf("messages = %#v", messages)
	}
	for i, got := range tags {
		if !reflect.DeepEqual(got, msg.Meta) {
			t.Fatalf("tags[%d] = %#v, want %#v", i, got, msg.Meta)
		}
	}
	for i, got := range levels {
		if got != sentry.LevelError {
			t.Fatalf("levels[%d] = %q, want %q", i, got, sentry.LevelError)
		}
	}
}

func TestSentryRejectsNilEventID(t *testing.T) {
	notifier := &sentryNotifier{capture: func(string, map[string]string, sentry.Level) *sentry.EventID { return nil }}
	if err := notifier.Notify(Message{}); err == nil {
		t.Fatal("Notify() error = nil, want rejected event error")
	}
}
