package notifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

func TestNewSlack(t *testing.T) {
	tests := []struct {
		name       string
		webhookURL string
		wantError  bool
	}{
		{name: "HTTPS URL", webhookURL: "https://hooks.slack.test/services/one/two/three"},
		{name: "HTTP URL", webhookURL: "http://localhost:8080/hook"},
		{name: "empty URL", wantError: true},
		{name: "relative URL", webhookURL: "/hook", wantError: true},
		{name: "unsupported scheme", webhookURL: "ftp://example.com/hook", wantError: true},
		{name: "missing host", webhookURL: "https:///hook", wantError: true},
		{name: "port without host", webhookURL: "http://:8080/hook", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewSlack(test.webhookURL)
			if (err != nil) != test.wantError {
				t.Fatalf("NewSlack(%q) error = %v, wantError %v", test.webhookURL, err, test.wantError)
			}
		})
	}
}

func TestSlackHTTPSuccess(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusAccepted, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			n, err := NewSlack(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if err := n.Notify(Message{}); err != nil {
				t.Fatalf("Notify() returned an error for HTTP %d: %v", status, err)
			}
		})
	}
}

func TestSlackNotifications(t *testing.T) {
	requests := make(chan slack.WebhookMessage, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload slack.WebhookMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- payload
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n, err := NewSlack(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	message := Message{
		Nanny:      "Home",
		Program:    "backup",
		NextSignal: 5 * time.Minute,
		Meta:       map[string]string{"region": "eu"},
	}
	tests := []struct {
		name      string
		notify    func(Message) error
		wantText  string
		wantColor string
	}{
		{
			name:      "alert",
			notify:    n.Notify,
			wantText:  message.Format(),
			wantColor: "#FF0000",
		},
		{
			name:      "all clear",
			notify:    n.NotifyAllClear,
			wantText:  message.FormatAllClear(),
			wantColor: "#00FF00",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.notify(message); err != nil {
				t.Fatal(err)
			}
			payload := <-requests
			if payload.Username != "Nanny" || payload.IconEmoji != ":baby_chick:" {
				t.Fatalf("unexpected identity: username=%q icon=%q", payload.Username, payload.IconEmoji)
			}
			if len(payload.Attachments) != 1 {
				t.Fatalf("got %d attachments, want 1", len(payload.Attachments))
			}
			attachment := payload.Attachments[0]
			if attachment.Text != test.wantText || attachment.Fallback != test.wantText {
				t.Fatalf("unexpected text: text=%q fallback=%q", attachment.Text, attachment.Fallback)
			}
			if attachment.Color != test.wantColor {
				t.Fatalf("color = %q, want %q", attachment.Color, test.wantColor)
			}
			if len(attachment.Fields) != 1 || attachment.Fields[0].Title != "region" || attachment.Fields[0].Value != "eu" {
				t.Fatalf("unexpected metadata fields: %#v", attachment.Fields)
			}
		})
	}
}

func TestSlackHTTPError(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			n, err := NewSlack(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if err := n.Notify(Message{}); err == nil {
				t.Fatalf("Notify() returned nil for HTTP %d", status)
			}
		})
	}
}

func TestSlackNetworkError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	webhookURL := server.URL
	server.Close()

	n, err := NewSlack(webhookURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Notify(Message{}); err == nil {
		t.Fatal("Notify() returned nil for a network error")
	}
}

func TestSlackRedirectError(t *testing.T) {
	redirectedRequests := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedRequests++
		w.WriteHeader(http.StatusOK)
	}))
	defer destination.Close()

	server := httptest.NewServer(http.RedirectHandler(destination.URL, http.StatusFound))
	defer server.Close()

	n, err := NewSlack(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Notify(Message{}); err == nil {
		t.Fatal("Notify() returned nil for a redirect response")
	}
	if redirectedRequests != 0 {
		t.Fatalf("redirect target received %d requests, want 0", redirectedRequests)
	}
}
