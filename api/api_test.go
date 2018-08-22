package api

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// TestAPINoNotifier tests if we correctly return error when the notifier we tried
// to use doesn't exist.
func TestAPINoNotifier(t *testing.T) {
	ts := serverSetup(t)
	defer ts.Close()

	payload := `{ "name": "my awesome program", "notifier": "N/A", "next_signal": "5s" }`
	resp, err := http.Post(ts.URL+"/api/v1/signal", "application/json", strings.NewReader(payload))
	require.NoError(t, err)
	require.NotNil(t, resp)

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	require.NoError(t, err)

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

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	require.NoError(t, err)

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

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	require.NoError(t, err)

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
