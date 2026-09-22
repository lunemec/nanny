package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // SQLite driver
)

const createSignalTable = `CREATE TABLE IF NOT EXISTS signal (
	name TEXT PRIMARY KEY NOT NULL,
	notifier TEXT NULL,
	next_signal DATETIME NULL,
	all_clear INTEGER DEFAULT 0 NULL,
	meta TEXT NULL
)`

type sqliteDB struct {
	db *sql.DB
}

// NewSQLiteDB creates new Storage using sqlite backend.
func NewSQLiteDB(dsn string) (Storage, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("unable to open sqlite database: %w", err)
	}
	if _, err := db.Exec(createSignalTable); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("unable to create sqlite table: %w", err)
	}
	if err := migrateSignalSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("unable to migrate sqlite table: %w", err)
	}
	if err := validateSignalSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("unable to validate sqlite table: %w", err)
	}

	return &sqliteDB{db: db}, nil
}

func migrateSignalSchema(db *sql.DB) error {
	columns, err := signalColumns(db)
	if err != nil {
		return err
	}

	for _, required := range []string{"name", "notifier", "next_signal"} {
		if !columns[required] {
			return fmt.Errorf("missing required column %q", required)
		}
	}
	if !columns["all_clear"] {
		if _, err := db.Exec("ALTER TABLE signal ADD COLUMN all_clear INTEGER DEFAULT 0 NULL"); err != nil {
			return err
		}
	}
	if !columns["meta"] {
		if _, err := db.Exec("ALTER TABLE signal ADD COLUMN meta TEXT NULL"); err != nil {
			return err
		}
	}
	return nil
}

func signalColumns(db *sql.DB) (columns map[string]bool, err error) {
	rows, err := db.Query("PRAGMA table_info(signal)")
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	columns = make(map[string]bool)
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func validateSignalSchema(db *sql.DB) (err error) {
	rows, err := db.Query("SELECT name, notifier, next_signal, all_clear, meta FROM signal LIMIT 0")
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	return nil
}

func (d *sqliteDB) Close() error {
	return d.db.Close()
}

func (d *sqliteDB) Load() (signals []Signal, err error) {
	rows, err := d.db.Query("SELECT name, notifier, CAST(next_signal AS TEXT), all_clear, meta FROM signal")
	if err != nil {
		return nil, fmt.Errorf("unable to load signals from sqlite: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("unable to close sqlite rows: %w", closeErr)
		}
	}()

	for rows.Next() {
		var (
			signal     Signal
			nextSignal string
			meta       []byte
		)
		if err := rows.Scan(&signal.Name, &signal.Notifier, &nextSignal, &signal.AllClear, &meta); err != nil {
			return nil, fmt.Errorf("unable to scan sqlite signal: %w", err)
		}

		signal.NextSignal, err = parseSQLiteTime(nextSignal)
		if err != nil {
			return nil, fmt.Errorf("unable to parse next signal for %q: %w", signal.Name, err)
		}
		if len(meta) > 0 {
			if err := json.Unmarshal(meta, &signal.Meta); err != nil {
				return nil, fmt.Errorf("unable to decode metadata for %q: %w", signal.Name, err)
			}
		}
		signals = append(signals, signal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("unable to iterate sqlite signals: %w", err)
	}

	return signals, nil
}

func (d *sqliteDB) Save(s Signal) error {
	meta, err := json.Marshal(s.Meta)
	if err != nil {
		return fmt.Errorf("unable to jsonify signal metadata: %w", err)
	}

	_, err = d.db.Exec(
		"INSERT OR REPLACE INTO signal (name, notifier, next_signal, all_clear, meta) VALUES (?, ?, ?, ?, ?)",
		s.Name,
		s.Notifier,
		s.NextSignal.UTC().Format(time.RFC3339Nano),
		s.AllClear,
		meta,
	)
	if err != nil {
		return fmt.Errorf("unable to save signal to sqlite: %w", err)
	}
	return nil
}

func (d *sqliteDB) Remove(s Signal) error {
	if s.Name == "" {
		return nil
	}
	if _, err := d.db.Exec("DELETE FROM signal WHERE name = ?", s.Name); err != nil {
		return fmt.Errorf("unable to remove sqlite record: %w", err)
	}
	return nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported sqlite timestamp %q", value)
}
