package postgres

import (
	"database/sql"
	"time"

	_ "github.com/lib/pq" //postgres drivers.
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type DB struct {
	Base   *sql.DB
	Logger *zap.Logger
}

type Config struct {
	URL             string
	MaxConnections  int
	MaxConnLifetime time.Duration
}

func New(logger *zap.Logger, cfg Config) (*DB, error) {
	db, err := sql.Open("postgres", cfg.URL)
	if err != nil {
		return nil, errors.Wrap(err, "can't open connection to postgres")
	}
	db.SetConnMaxLifetime(cfg.MaxConnLifetime)
	db.SetMaxIdleConns(cfg.MaxConnections)
	db.SetMaxOpenConns(cfg.MaxConnections)

	return &DB{
		Base:   db,
		Logger: logger,
	}, nil
}

func (db *DB) CheckConnection() error {
	var err error
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = db.Base.Ping(); err == nil {
			break
		}
		nextAttemptWait := time.Duration(attempt) * time.Second
		db.Logger.Sugar().Errorf("attempt %d: can't establish a connection with the db, wait for %v: %s",
			attempt,
			nextAttemptWait,
			err,
		)
		time.Sleep(nextAttemptWait)
	}
	return errors.Wrap(err, "can't connect to db")
}

func (db *DB) Close() error {
	if err := db.Base.Close(); err != nil {
		return errors.Wrap(err, "can't close db")
	}
	return nil
}

type sqlScanner interface {
	Scan(dest ...interface{}) error
}

type sqlMultiScanner interface {
	Scan(dest ...interface{}) error
	Next() bool
}
