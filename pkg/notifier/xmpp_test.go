package notifier

import (
	"errors"
	"testing"
	"time"

	"github.com/xmppo/go-xmpp"
)

func TestNewXmppValidation(t *testing.T) {
	tests := []struct {
		name   string
		to     []string
		server string
		user   string
	}{
		{name: "recipients", server: "xmpp.example.com", user: "nanny"},
		{name: "server", to: []string{"ops@example.com"}, user: "nanny"},
		{name: "user", to: []string{"ops@example.com"}, server: "xmpp.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewXmpp(tt.to, tt.server, 5222, tt.user, "password", "Nanny", false); err == nil {
				t.Fatal("NewXmpp() error = nil, want validation error")
			}
		})
	}
}

func TestXmppAlertAllClearAndClose(t *testing.T) {
	notifierValue, err := NewXmpp(
		[]string{"one@example.com", "two@example.com"},
		"xmpp.example.com",
		5222,
		"nanny@example.com",
		"password",
		"Nanny",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	notifier := notifierValue.(*xmppNotifier)
	var options []xmpp.Options
	var chats []xmpp.Chat
	closed := 0
	notifier.open = func(option xmpp.Options) (xmppSession, error) {
		options = append(options, option)
		return xmppSession{
			send: func(chat xmpp.Chat) (int, error) {
				chats = append(chats, chat)
				return 1, nil
			},
			close: func() error {
				closed++
				return nil
			},
		}, nil
	}
	msg := Message{Nanny: "Nanny", Program: "cron", NextSignal: time.Minute, Meta: map[string]string{"env": "test"}}

	if err := notifier.Notify(msg); err != nil {
		t.Fatal(err)
	}
	if err := notifier.NotifyAllClear(msg); err != nil {
		t.Fatal(err)
	}
	if len(options) != 2 || len(chats) != 4 || closed != 2 {
		t.Fatalf("options=%d chats=%d closes=%d", len(options), len(chats), closed)
	}
	option := options[0]
	if option.Host != "xmpp.example.com:5222" || option.User != notifier.User || option.Password != notifier.Password || option.Resource != notifier.Resource {
		t.Fatalf("options = %+v", option)
	}
	if option.DialTimeout != 10*time.Second || !option.NoTLS || !option.InsecureAllowUnencryptedAuth {
		t.Fatalf("security options = %+v", option)
	}
	if chats[0].Remote != "one@example.com" || chats[1].Remote != "two@example.com" {
		t.Fatalf("chats = %+v", chats)
	}
	if chats[0].Text == chats[2].Text {
		t.Fatalf("alert and all-clear text are equal: %q", chats[0].Text)
	}
}

func TestXmppSendFailureStillCloses(t *testing.T) {
	wantErr := errors.New("send failed")
	closed := false
	notifier := &xmppNotifier{
		To: []string{"ops@example.com"},
		open: func(xmpp.Options) (xmppSession, error) {
			return xmppSession{
				send:  func(xmpp.Chat) (int, error) { return 0, wantErr },
				close: func() error { closed = true; return nil },
			}, nil
		},
	}
	if err := notifier.Notify(Message{}); !errors.Is(err, wantErr) {
		t.Fatalf("Notify() error = %v, want %v", err, wantErr)
	}
	if !closed {
		t.Fatal("XMPP session was not closed after send failure")
	}
}

func TestXmppCloseFailure(t *testing.T) {
	wantErr := errors.New("close failed")
	notifier := &xmppNotifier{
		open: func(xmpp.Options) (xmppSession, error) {
			return xmppSession{
				send:  func(xmpp.Chat) (int, error) { return 0, nil },
				close: func() error { return wantErr },
			}, nil
		},
	}
	if err := notifier.Notify(Message{}); !errors.Is(err, wantErr) {
		t.Fatalf("Notify() error = %v, want %v", err, wantErr)
	}
}

func TestXmppConnectFailure(t *testing.T) {
	wantErr := errors.New("connect failed")
	notifier := &xmppNotifier{
		open: func(xmpp.Options) (xmppSession, error) { return xmppSession{}, wantErr },
	}
	if err := notifier.Notify(Message{}); !errors.Is(err, wantErr) {
		t.Fatalf("Notify() error = %v, want %v", err, wantErr)
	}
}
