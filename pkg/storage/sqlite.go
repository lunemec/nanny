package storage

import (
	"encoding/json"

	"github.com/go-xorm/xorm"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"github.com/pkg/errors"
)

type sqliteDB struct {
	db *xorm.Engine
}

// NewSQLiteDB creates new Storage using sqlite backend.
func NewSQLiteDB(dsn string) (Storage, error) {
	engine, err := xorm.NewEngine("sqlite3", dsn)
	if err != nil {
		return nil, errors.Wrap(err, "unable to open sqlite database")
	}

	err = engine.Sync2(new(Signal))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create sqlite table")
	}
	return &sqliteDB{db: engine}, nil
}

func (d *sqliteDB) Close() error {
	return d.db.Close()
}

func (d *sqliteDB) Load() ([]Signal, error) {
	var signals []Signal
	err := d.db.Find(&signals)
	if err != nil {
		return signals, errors.Wrap(err, "unable to load signals from sqlite")
	}

	return signals, nil
}

func (d *sqliteDB) Save(s Signal) error {
	sql := "INSERT OR REPLACE INTO `signal` (name, notifier, next_signal, meta) VALUES (?, ?, ?, ?)"

	meta, err := json.Marshal(s.Meta)
	if err != nil {
		return errors.Wrap(err, "unable to jsonify signal metadata")
	}
	_, err = d.db.Exec(sql, s.Name, s.Notifier, s.NextSignal.UTC(), meta)
	if err != nil {
		return errors.Wrapf(err, "unable to save signal to sqlite: %+v", s)
	}
	return nil
}

func (d *sqliteDB) Remove(s Signal) error {
	if s.Name == "" {
		return nil
	}
	_, err := d.db.Id(s.Name).Delete(&Signal{})
	if err != nil {
		return errors.Wrapf(err, "unable to remove sqlite record: %+v", s)
	}
	return nil
}
