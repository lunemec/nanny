package notifier

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/pkg/errors"
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
		return nil, errors.New("Unable to initialize webhook: webhookURL is empty")
	}
	if WebhookURLAllClear == "" {
		return nil, errors.New("Unable to initialize webhook: webhookURL_all_clear is empty")
	}

	httpClient := &http.Client{Timeout: time.Second * RequestTimeout}
	if AllowInsecureTLS {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{Transport: transport, Timeout: time.Second * RequestTimeout}
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
	postBody, _ := json.Marshal(map[string]interface{}{
		"message": msg.Format(),
		"meta":    msg.Meta,
	})
	request, err := http.NewRequest("POST", w.WebhookURL, bytes.NewBuffer(postBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Program", msg.Program)

	if w.WebhookSecret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		payload := append([]byte(timestamp), postBody...)
		signature := ComputeHmacSha256(w.WebhookSecret, payload)

		request.Header.Set("X-Timestamp", timestamp)
		request.Header.Set("X-HMAC-SHA256", signature)
	}

	_, err = w.httpClient.Do(request)
	if err != nil {
		return errors.Wrap(err, "unable to notify via webhook")
	}

	return nil
}

// NotifyAllClear implements the Notifier interface for webhook.
func (w *webhookNotifier) NotifyAllClear(msg Message) error {
	postBody, _ := json.Marshal(map[string]interface{}{
		"message": msg.FormatAllClear(),
		"meta":    msg.Meta,
	})
	request, err := http.NewRequest("POST", w.WebhookURLAllClear, bytes.NewBuffer(postBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Program", msg.Program)

	if w.WebhookSecret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		payload := append([]byte(timestamp), postBody...)
		signature := ComputeHmacSha256(w.WebhookSecret, payload)

		request.Header.Set("X-Timestamp", timestamp)
		request.Header.Set("X-HMAC-SHA256", signature)
	}

	_, err = w.httpClient.Do(request)
	if err != nil {
		return errors.Wrap(err, "unable to notify via webhook")
	}

	return nil
}

func (w *webhookNotifier) String() string {
	return "webhook"
}
