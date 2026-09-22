package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nanny/pkg/nanny"
	"nanny/pkg/notifier"
	"nanny/pkg/storage"
	"nanny/pkg/version"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DummyNotifier's only job is to store `msg` argument
// when `Notify()` method is called.
type DummyNotifier struct {
	notifyMsg notifier.Message
	lock      sync.Mutex
}

// Notify stores `msg` in the DummyNotifier.
func (d *DummyNotifier) Notify(msg notifier.Message) error {
	d.lock.Lock()
	d.notifyMsg = msg
	d.lock.Unlock()
	return nil
}

// NotifyAllClear stores `msg` in the DummyNotifier.
func (d *DummyNotifier) NotifyAllClear(msg notifier.Message) error {
	d.lock.Lock()
	d.notifyMsg = msg
	d.lock.Unlock()
	return nil
}

func (d *DummyNotifier) String() string {
	return "dummy"
}

// NotifyMsg retrieves `msg` argument from previous `Notify` call. For testing
// purposes only.
func (d *DummyNotifier) NotifyMsg() notifier.Message {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.notifyMsg
}

type testStorage struct{}

func (s *testStorage) Load() ([]storage.Signal, error) { return nil, nil }
func (s *testStorage) Save(storage.Signal) error       { return nil }
func (s *testStorage) Remove(storage.Signal) error     { return nil }
func (s *testStorage) Close() error                    { return nil }

type orderedStorage struct {
	firstStarted chan struct{}
	releaseFirst chan struct{}
	secondSaved  chan struct{}
	mu           sync.Mutex
	saves        []string
}

type expiryStorage struct {
	saveStarted chan struct{}
	releaseSave chan struct{}
	removed     chan struct{}
	mu          sync.Mutex
	events      []string
}

func (s *expiryStorage) Load() ([]storage.Signal, error) { return nil, nil }
func (s *expiryStorage) Close() error                    { return nil }
func (s *expiryStorage) Save(storage.Signal) error {
	close(s.saveStarted)
	<-s.releaseSave
	s.mu.Lock()
	s.events = append(s.events, "save")
	s.mu.Unlock()
	return nil
}
func (s *expiryStorage) Remove(storage.Signal) error {
	s.mu.Lock()
	s.events = append(s.events, "remove")
	s.mu.Unlock()
	close(s.removed)
	return nil
}

type blockingAllClearNotifier struct {
	alerted         chan struct{}
	releaseAlert    chan struct{}
	allClearStarted chan struct{}
	releaseAllClear chan struct{}
}

func (n *blockingAllClearNotifier) Notify(notifier.Message) error {
	close(n.alerted)
	<-n.releaseAlert
	return nil
}
func (n *blockingAllClearNotifier) NotifyAllClear(notifier.Message) error {
	close(n.allClearStarted)
	<-n.releaseAllClear
	return nil
}
func (n *blockingAllClearNotifier) String() string { return "blocking" }

func (s *orderedStorage) Load() ([]storage.Signal, error) { return nil, nil }
func (s *orderedStorage) Remove(storage.Signal) error     { return nil }
func (s *orderedStorage) Close() error                    { return nil }
func (s *orderedStorage) Save(signal storage.Signal) error {
	request := signal.Meta["request"]
	if request == "first" {
		close(s.firstStarted)
		<-s.releaseFirst
	}
	s.mu.Lock()
	s.saves = append(s.saves, request)
	s.mu.Unlock()
	if request == "second" {
		close(s.secondSaved)
	}
	return nil
}

var dummy = DummyNotifier{}
var testNotifiers = notifiers{"dummy": &dummy}

func nannySetup(t *testing.T) *nanny.Nanny {
	t.Helper()
	return &nanny.Nanny{}
}

func storageSetup(t *testing.T) storage.Storage {
	t.Helper()
	return &testStorage{}
}

func routerSetup(t *testing.T) http.Handler {
	t.Helper()
	return router(nannySetup(t), testNotifiers, storageSetup(t))
}

func serverSetup(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(routerSetup(t))
}

func readResponseBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return body
}

func TestAPIVersion(t *testing.T) {
	assert.HTTPBodyContains(t, routerSetup(t).ServeHTTP, "GET", "/api/version", url.Values{}, version.VersionString)
}

func TestListEndpoints(t *testing.T) {
	got := assert.HTTPBody(routerSetup(t).ServeHTTP, "GET", "/api/", url.Values{})
	expected := `{
		"/api":"",
		"/api/":"List all available API endpoints.",
		"/api/v1":"",
		"/api/v1/signal":"Register new signal.",
		"/api/v1/signals":"Show all registered signals.",
		"/api/version":"Nanny version."
	}`
	assert.JSONEq(t, expected, got)
}

func TestHTTPErrorPreservesWrappedError(t *testing.T) {
	wantErr := errors.New("sentinel")
	err := &httpError{StatusCode: http.StatusBadRequest, Err: wantErr}
	if !errors.Is(err, wantErr) {
		t.Fatalf("errors.Is(%v, %v) = false", err, wantErr)
	}
}

func TestHandlerRestoresPersistedSignal(t *testing.T) {
	store, err := storage.NewSQLiteDB(filepath.Join(t.TempDir(), "nanny.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.Save(storage.Signal{
		Name:       "restored program",
		Notifier:   "dummy",
		NextSignal: time.Now().Add(time.Hour),
		AllClear:   true,
		Meta:       map[string]string{"source": "restart"},
	}))

	server := Server{Notifiers: testNotifiers, Storage: store}
	handler, err := server.Handler()
	require.NoError(t, err)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/signals", nil))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "restored program")
	assert.Contains(t, response.Body.String(), `"source":"restart"`)
}

func TestSignalPersistenceFollowsTimerUpdateOrder(t *testing.T) {
	store := &orderedStorage{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
		secondSaved:  make(chan struct{}),
	}
	handler := router(&nanny.Nanny{}, testNotifiers, store)
	send := func(request string) <-chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			body := strings.NewReader(`{"name":"program","notifier":"dummy","next_signal":"1h","meta":{"request":"` + request + `"}}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/signal", body)
			req.Header.Set("X-Dont-Modify-Name", "true")
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}()
		return done
	}

	firstDone := send("first")
	<-store.firstStarted
	secondDone := send("second")
	select {
	case <-store.secondSaved:
		t.Fatal("second request persisted before the first request completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(store.releaseFirst)
	<-firstDone
	<-secondDone

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Equal(t, []string{"first", "second"}, store.saves)
}

func TestSignalPersistenceCompletesBeforeTimerExpiry(t *testing.T) {
	store := &expiryStorage{
		saveStarted: make(chan struct{}),
		releaseSave: make(chan struct{}),
		removed:     make(chan struct{}),
	}
	handler := router(&nanny.Nanny{}, testNotifiers, store)
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/signal", strings.NewReader(
			`{"name":"expiring","notifier":"dummy","next_signal":"10ms"}`,
		))
		req.Header.Set("X-Dont-Modify-Name", "true")
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}()
	<-store.saveStarted

	select {
	case <-store.removed:
		t.Fatal("timer expired before its persistence completed")
	case <-time.After(30 * time.Millisecond):
	}
	close(store.releaseSave)
	<-done
	select {
	case <-store.removed:
	case <-time.After(time.Second):
		t.Fatal("timer did not expire after persistence completed")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Equal(t, []string{"save", "remove"}, store.events)
}

func TestSlowAllClearDoesNotBlockOtherSignals(t *testing.T) {
	slow := &blockingAllClearNotifier{
		alerted:         make(chan struct{}),
		releaseAlert:    make(chan struct{}),
		allClearStarted: make(chan struct{}),
		releaseAllClear: make(chan struct{}),
	}
	release := func(channel chan struct{}) {
		select {
		case <-channel:
		default:
			close(channel)
		}
	}
	defer release(slow.releaseAlert)
	defer release(slow.releaseAllClear)
	handler := router(&nanny.Nanny{}, notifiers{"slow": slow, "dummy": &DummyNotifier{}}, &testStorage{})
	send := func(name, notifierName, nextSignal string, allClear bool) <-chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			body := strings.NewReader(fmt.Sprintf(
				`{"name":%q,"notifier":%q,"next_signal":%q,"all_clear":%t}`,
				name, notifierName, nextSignal, allClear,
			))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/signal", body)
			req.Header.Set("X-Dont-Modify-Name", "true")
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}()
		return done
	}

	<-send("slow", "slow", "10ms", true)
	<-slow.alerted
	slowHeartbeat := send("slow", "slow", "1h", true)
	select {
	case <-slowHeartbeat:
	case <-time.After(time.Second):
		t.Fatal("blocked alert delayed the heartbeat response")
	}
	select {
	case <-slow.allClearStarted:
		t.Fatal("all-clear overtook the blocked alert")
	case <-time.After(20 * time.Millisecond):
	}
	release(slow.releaseAlert)
	select {
	case <-slow.allClearStarted:
	case <-time.After(time.Second):
		t.Fatal("all-clear did not follow the alert")
	}

	sameHeartbeat := send("slow", "slow", "1h", true)
	select {
	case <-sameHeartbeat:
	case <-time.After(time.Second):
		t.Fatal("blocked all-clear delayed the heartbeat response")
	}
	otherHeartbeat := send("other", "dummy", "1h", false)
	select {
	case <-otherHeartbeat:
	case <-time.After(time.Second):
		t.Fatal("slow all-clear blocked an unrelated signal")
	}
	release(slow.releaseAllClear)
}

func TestConstructSignalReservesCallbackForConsumers(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signal", nil)
	req.Header.Set("X-Dont-Modify-Name", "true")
	signal := constructSignal(Signal{Name: "program", NextSignal: "1h"}, &DummyNotifier{}, req)
	if signal.CallbackFunc != nil {
		t.Fatal("constructSignal() set a persistence callback on the public consumer callback")
	}
}

// TestAPINoNotifier tests if we correctly return error when the notifier we tried
// to use doesn't exist.
func TestAPINoNotifier(t *testing.T) {
	ts := serverSetup(t)
	defer ts.Close()

	payload := `{ "name": "my awesome program", "notifier": "N/A", "next_signal": "5s" }`
	resp, err := http.Post(ts.URL+"/api/v1/signal", "application/json", strings.NewReader(payload))
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer func() { require.NoError(t, resp.Body.Close()) }()

	body := readResponseBody(t, resp)

	assert.Equal(t, 400, resp.StatusCode)
	expected := `{"status_code":400, "error":"unable to find notifier: N/A"}`
	assert.JSONEq(t, expected, string(body))
}

// TestAPISignal tests correct error emit when API isn't called within specified
// time.
func TestAPISignal(t *testing.T) {
	ts := serverSetup(t)
	defer ts.Close()

	payload := `{ "name": "my awesome program", "notifier": "dummy", "next_signal": "1s" }`
	resp, err := http.Post(ts.URL+"/api/v1/signal", "application/json", strings.NewReader(payload))
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer func() { require.NoError(t, resp.Body.Close()) }()

	body := readResponseBody(t, resp)

	assert.Equal(t, 200, resp.StatusCode)
	expected := `{"status_code":200, "status":"OK"}`
	assert.JSONEq(t, expected, string(body))

	// Wait for the timer to expire and check if dummyNotifier has something for us.
	time.Sleep(1100 * time.Millisecond)
	msg := dummy.NotifyMsg()
	assert.Contains(t, msg.Format(), `Nanny: I did not hear from "my awesome program@127.0.0.1" in 1s!`)
}

// TestAPISignalAcceptsInt test that "next_signal" can be string in seconds.
func TestAPISignalAcceptsInt(t *testing.T) {
	ts := serverSetup(t)
	defer ts.Close()

	payload := `{ "name": "my awesome program", "notifier": "dummy", "next_signal": "1" }`
	resp, err := http.Post(ts.URL+"/api/v1/signal", "application/json", strings.NewReader(payload))
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer func() { require.NoError(t, resp.Body.Close()) }()

	body := readResponseBody(t, resp)

	assert.Equal(t, 200, resp.StatusCode)
	expected := `{"status_code":200, "status":"OK"}`
	assert.JSONEq(t, expected, string(body))

	// Wait for the timer to expire and check if dummyNotifier has something for us.
	time.Sleep(1100 * time.Millisecond)
	msg := dummy.NotifyMsg()
	assert.Contains(t, msg.Format(), `Nanny: I did not hear from "my awesome program@127.0.0.1" in 1s!`)
}

// TODO
func TestPersistence(t *testing.T) {}

func TestConstructName(t *testing.T) {
	signalName := "test_name"
	r, _ := http.NewRequest("POST", "/ignored/anyway", nil)
	r.RemoteAddr = "10.11.12.13:8089"

	assert.Equal(t, constructName(signalName, r), "test_name@10.11.12.13")
}

func TestConstructNameXForwardedForHeader(t *testing.T) {
	signalName := "test_name_x_forwarded_for"
	r, _ := http.NewRequest("POST", "/ignored/anyway", nil)
	r.RemoteAddr = "10.11.12.13:8089"
	r.Header.Add("X-Forwarded-For", "14.15.16.17")

	assert.Equal(t, constructName(signalName, r), "test_name_x_forwarded_for@14.15.16.17")
}
func TestConstructNameXDontModifyNameHeader(t *testing.T) {
	signalName := "test_name_x_dont_modify_name"
	r, _ := http.NewRequest("POST", "/ignored/anyway", nil)
	r.RemoteAddr = "10.11.12.13:8089"
	r.Header.Add("X-Forwarded-For", "14.15.16.17")
	r.Header.Add("X-Dont-Modify-Name", "true")

	assert.Equal(t, constructName(signalName, r), "test_name_x_dont_modify_name")
}
