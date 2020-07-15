package session

import (
	"time"
)

type Session struct {
	SessionID  string
	UserID     int64
	CreatedAt  time.Time
	ValidUntil time.Time
}

type Storage interface {
	Create(sess *Session) error
	DeleteByUserID(userID int64) error
	GetByUserID(userID int64) (*Session, error)
	GetByBearer(bearer string) (*Session, error)
}
