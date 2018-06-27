package storage_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"nanny/pkg/storage"
)

var sqliteStorage storage.Storage

func TestMain(m *testing.M) {
	var err error
	sqliteStorage, err = storage.NewSQLiteDB("file::memory:")
	if err != nil {
		fmt.Printf("Error setting up storage test: %s\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// TestSQLiteDataConsistency tests that data we saved will be loaded the same. There
// were some issues with timezones.
func TestSQLiteDataConsistency(t *testing.T) {
	signal := storage.Signal{
		Name:       "test",
		NextSignal: time.Now(),
		Notifier:   "stderr",
		Meta:       map[string]string{"meta": "data"},
	}
	err := sqliteStorage.Save(signal)
	if err != nil {
		t.Errorf("signal save failed: %s", err)
	}

	signals, err := sqliteStorage.Load()
	if err != nil {
		t.Errorf("signal load failed: %s", err)
	}

	if len(signals) != 1 {
		t.Error("there should be exactly 1 signal loaded")
	}

	compareSignals(t, signal, signals[0])
}

func compareSignals(t *testing.T, this storage.Signal, other storage.Signal) {
	if this.Name != other.Name {
		t.Errorf("saved signal is not equal to loaded signal, saved: %+v, loaded: %+v", this.Name, other.Name)
	}

	// We have to strip monotonic clock readings before comparing.
	// See: https://golang.org/pkg/time/#hdr-Monotonic_Clocks
	if this.NextSignal.Round(0) != other.NextSignal {
		t.Errorf("saved signal is not equal to loaded signal, saved: %+v, loaded: %+v", this.NextSignal, other.NextSignal)
	}

	if this.Notifier != other.Notifier {
		t.Errorf("saved signal is not equal to loaded signal, saved: %+v, loaded: %+v", this.Notifier, other.Notifier)
	}

	if this.Meta["meta"] != other.Meta["meta"] {
		t.Errorf("saved signal is not equal to loaded signal, saved: %+v, loaded: %+v", this.Meta, other.Meta)
	}
}
