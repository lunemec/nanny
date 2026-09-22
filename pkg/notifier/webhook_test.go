package notifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebhookTimeoutAndMessages(t *testing.T) {
	type request struct {
		path    string
		message string
	}
	requests := make(chan request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- request{path: r.URL.Path, message: body.Message}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	n, err := NewWebhook(server.URL+"/alert", server.URL+"/clear", "", 10*time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	webhook := n.(*webhookNotifier)
	if webhook.httpClient.Timeout != 10*time.Second {
		t.Fatalf("HTTP timeout = %v, want 10s", webhook.httpClient.Timeout)
	}
	msg := Message{Program: "cron", NextSignal: time.Minute}
	if err := webhook.Notify(msg); err != nil {
		t.Fatal(err)
	}
	if err := webhook.NotifyAllClear(msg); err != nil {
		t.Fatal(err)
	}
	alert, clear := <-requests, <-requests
	if alert.path != "/alert" || !strings.Contains(alert.message, "did not hear") {
		t.Fatalf("alert request = %+v", alert)
	}
	if clear.path != "/clear" || !strings.Contains(clear.message, "did hear") {
		t.Fatalf("all-clear request = %+v", clear)
	}
}

func TestWebhookRejectsHTTPFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed", http.StatusBadGateway)
	}))
	defer server.Close()

	n, err := NewWebhook(server.URL, server.URL, "", time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	for name, notify := range map[string]func(Message) error{
		"alert":     n.Notify,
		"all-clear": n.NotifyAllClear,
	} {
		t.Run(name, func(t *testing.T) {
			if err := notify(Message{}); err == nil || !strings.Contains(err.Error(), "502") {
				t.Fatalf("notification error = %v, want HTTP 502", err)
			}
		})
	}
}
