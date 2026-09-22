package notifier

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type webhookNotifier struct {
	WebhookURL         string
	WebhookURLAllClear string
	WebhookSecret      string
	httpClient         *http.Client
}

func ComputeHmacSha256(secret string, payload []byte) string {
	key := []byte(secret)
	h := hmac.New(sha256.New, key)
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// NewWebhook creates a new webhook notifier from the supplied configuration.
func NewWebhook(WebhookURL string,
	WebhookURLAllClear string,
	WebhookSecret string,
	RequestTimeout time.Duration,
	AllowInsecureTLS bool) (Notifier, error) {

	if WebhookURL == "" {
		return nil, errors.New("unable to initialize webhook: webhookURL is empty")
	}
	if WebhookURLAllClear == "" {
		return nil, errors.New("unable to initialize webhook: webhookURL_all_clear is empty")
	}

	httpClient := &http.Client{Timeout: RequestTimeout}
	if AllowInsecureTLS {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{Transport: transport, Timeout: RequestTimeout}
	}

	return &webhookNotifier{
		WebhookURL,
		WebhookURLAllClear,
		WebhookSecret,
		httpClient,
	}, nil
}

// Notify implements the Notifier interface for webhook.
func (w *webhookNotifier) Notify(msg Message) error {
	return w.notify(msg, false)
}

// NotifyAllClear implements the Notifier interface for webhook.
func (w *webhookNotifier) NotifyAllClear(msg Message) error {
	return w.notify(msg, true)
}

func (w *webhookNotifier) notify(msg Message, allClear bool) error {
	webhookURL := w.WebhookURL
	message := msg.Format()
	if allClear {
		webhookURL = w.WebhookURLAllClear
		message = msg.FormatAllClear()
	}
	postBody, _ := json.Marshal(map[string]any{
		"message": message,
		"meta":    msg.Meta,
	})
	request, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(postBody))
	if err != nil {
		return fmt.Errorf("unable to create webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Program", msg.Program)

	if w.WebhookSecret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		payload := append([]byte(timestamp), postBody...)
		signature := ComputeHmacSha256(w.WebhookSecret, payload)

		request.Header.Set("X-Timestamp", timestamp)
		request.Header.Set("X-HMAC-SHA256", signature)
	}

	response, err := w.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("unable to notify via webhook: %w", err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if readErr != nil {
		return fmt.Errorf("unable to read webhook response: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("unable to close webhook response: %w", closeErr)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("webhook returned HTTP status %s", response.Status)
	}

	return nil
}

func (w *webhookNotifier) String() string {
	return "webhook"
}
