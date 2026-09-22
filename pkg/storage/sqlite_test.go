package storage_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"nanny/pkg/storage"
)

func newStorage(t *testing.T, dsn string) storage.Storage {
	t.Helper()
	store, err := storage.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatalf("NewSQLiteDB() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}

func TestSQLiteSaveUpdateLoadRemove(t *testing.T) {
	store := newStorage(t, "file:save-update?mode=memory&cache=shared")
	nextSignal := time.Date(2026, time.September, 22, 12, 34, 56, 123456789, time.FixedZone("CEST", 2*60*60))
	signal := storage.Signal{
		Name:       "test",
		NextSignal: nextSignal,
		Notifier:   "stderr",
		AllClear:   true,
		Meta:       map[string]string{"meta": "data"},
	}

	if err := store.Save(signal); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	signal.Notifier = "slack"
	signal.Meta["updated"] = "yes"
	if err := store.Save(signal); err != nil {
		t.Fatalf("Save() update error = %v", err)
	}

	signals, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("Load() returned %d signals, want 1", len(signals))
	}
	got := signals[0]
	if got.Name != signal.Name || got.Notifier != signal.Notifier || got.AllClear != signal.AllClear {
		t.Fatalf("Load() = %+v, want %+v", got, signal)
	}
	if !got.NextSignal.Equal(nextSignal) {
		t.Fatalf("NextSignal = %v, want %v", got.NextSignal, nextSignal)
	}
	if got.NextSignal.Location() != time.UTC {
		t.Fatalf("NextSignal location = %v, want UTC", got.NextSignal.Location())
	}
	if got.Meta["meta"] != "data" || got.Meta["updated"] != "yes" {
		t.Fatalf("Meta = %#v", got.Meta)
	}

	if err := store.Remove(signal); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := store.Remove(storage.Signal{}); err != nil {
		t.Fatalf("Remove(empty) error = %v", err)
	}
	signals, err = store.Load()
	if err != nil {
		t.Fatalf("Load() after remove error = %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("Load() after remove returned %d signals, want 0", len(signals))
	}
}

func TestSQLiteLoadsOldSchemaAndTimestamp(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "old.sqlite")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE signal (
		name TEXT PRIMARY KEY NOT NULL,
		notifier TEXT NULL,
		next_signal DATETIME NULL,
		meta TEXT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(
		"INSERT INTO signal (name, notifier, next_signal, meta) VALUES (?, ?, ?, ?)",
		"legacy", "stderr", "2021-07-08 09:10:11.123456789+02:00", `{"legacy":"metadata"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store := newStorage(t, dsn)
	signals, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("Load() returned %d signals, want 1", len(signals))
	}
	wantTime := time.Date(2021, time.July, 8, 9, 10, 11, 123456789, time.FixedZone("", 2*60*60))
	if !signals[0].NextSignal.Equal(wantTime) || signals[0].AllClear || signals[0].Meta["legacy"] != "metadata" {
		t.Fatalf("Load() = %+v", signals[0])
	}
	signals[0].AllClear = true
	if err := store.Save(signals[0]); err != nil {
		t.Fatalf("Save() after migration error = %v", err)
	}
}

func TestSQLiteFilePersistence(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "nanny.sqlite")
	store, err := storage.NewSQLiteDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	want := storage.Signal{
		Name:       "persisted",
		Notifier:   "stderr",
		NextSignal: time.Now().Add(time.Hour).Round(0),
		Meta:       map[string]string{"restart": "restored"},
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := newStorage(t, dsn)
	got, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != want.Name || !got[0].NextSignal.Equal(want.NextSignal) || got[0].Meta["restart"] != "restored" {
		t.Fatalf("Load() after reopen = %+v, want %+v", got, want)
	}
}

func TestSQLiteMalformedRows(t *testing.T) {
	tests := []struct {
		name       string
		notifier   any
		nextSignal any
		meta       any
	}{
		{name: "scan", notifier: nil, nextSignal: "2026-09-22T12:34:56Z", meta: `{}`},
		{name: "timestamp", notifier: "stderr", nextSignal: "not-a-time", meta: `{}`},
		{name: "metadata", notifier: "stderr", nextSignal: "2026-09-22T12:34:56Z", meta: `{not-json}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := "file:malformed-" + tt.name + "?mode=memory&cache=shared"
			store := newStorage(t, dsn)
			db, err := sql.Open("sqlite", dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := db.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			}()
			_, err = db.Exec(
				"INSERT INTO signal (name, notifier, next_signal, all_clear, meta) VALUES (?, ?, ?, ?, ?)",
				tt.name, tt.notifier, tt.nextSignal, 0, tt.meta,
			)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := store.Load(); err == nil {
				t.Fatal("Load() error = nil, want malformed row error")
			}
		})
	}
}

func TestSQLiteConstructorErrors(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "missing", "nanny.sqlite")
	if _, err := storage.NewSQLiteDB(dsn); err == nil {
		t.Fatal("NewSQLiteDB() error = nil, want open error")
	}
}

func TestSQLiteSchemaErrors(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "invalid-schema.sqlite")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE VIEW signal AS SELECT 1 AS value"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := storage.NewSQLiteDB(dsn); err == nil {
		t.Fatal("NewSQLiteDB() error = nil, want schema error")
	}
}
