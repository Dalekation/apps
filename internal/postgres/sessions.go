package postgres

import (
	"database/sql"
	sessions "finPrj/internal/sessions"

	"github.com/pkg/errors"
)

var _ sessions.Storage = &SessionStorage{}

type SessionStorage struct {
	statementStorage

	CreateStmt         *sql.Stmt
	DeleteByUserIDStmt *sql.Stmt
	GetByUserIDStmt    *sql.Stmt
	GetByBearerStmt    *sql.Stmt
}

func NewSessionStorage(db *DB) (*SessionStorage, error) {
	ss := &SessionStorage{statementStorage: newStatementsStorage(db)}

	stmts := []stmt{
		{Query: createSessionQuery, Dst: &ss.CreateStmt},
		{Query: deleteByUserIDQuery, Dst: &ss.DeleteByUserIDStmt},
		{Query: getByUserIDQuery, Dst: &ss.GetByUserIDStmt},
		{Query: getByBearerQuery, Dst: &ss.GetByBearerStmt},
	}

	if err := ss.initStatements(stmts); err != nil {
		return nil, errors.Wrap(err, "can't init statements in sessions")
	}

	return ss, nil
}

const createSessionQuery = `INSERT INTO sessions (session_id, user_id, created_at, valid_until) VALUES ($1, $2, $3, $4)`

func (ss *SessionStorage) Create(sess *sessions.Session) error {
	_, err := ss.CreateStmt.Exec(sess.SessionID, sess.UserID, sess.CreatedAt, sess.ValidUntil)
	if err != nil {
		return errors.Wrapf(err, "can't create session")
	}

	return nil
}

const deleteByUserIDQuery = `DELETE FROM sessions WHERE user_id = $1`

func (ss *SessionStorage) DeleteByUserID(userID int64) error {
	_, err := ss.DeleteByUserIDStmt.Exec(userID)
	if err != nil {
		return errors.Wrapf(err, "can't delete session")
	}

	return nil
}

const getByUserIDQuery = `SELECT session_id, user_id, created_at, valid_until FROM sessions WHERE user_id = $1`

func (ss *SessionStorage) GetByUserID(userID int64) (*sessions.Session, error) {
	row := ss.GetByUserIDStmt.QueryRow(userID)
	sess := sessions.Session{}

	err := row.Scan(&sess.SessionID, &sess.UserID, &sess.CreatedAt, &sess.ValidUntil)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "can't get session by user id")
	}

	return &sess, nil
}

const getByBearerQuery = `SELECT session_id, user_id, created_at, valid_until FROM sessions WHERE session_id = $1`

func (ss *SessionStorage) GetByBearer(bearer string) (*sessions.Session, error) {
	row := ss.GetByBearerStmt.QueryRow(bearer)
	sess := sessions.Session{}

	err := row.Scan(&sess.SessionID, &sess.UserID, &sess.CreatedAt, &sess.ValidUntil)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "can't get session by bearer")
	}

	return &sess, nil
}
