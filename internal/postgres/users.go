package postgres

import (
	"database/sql"
	"time"

	users "finPrj/internal/users"

	"github.com/pkg/errors"
)

var _ users.Storage = &UserStorage{}

type UserStorage struct {
	statementStorage

	CreateStmt     *sql.Stmt
	GetByIDStmt    *sql.Stmt
	GetByEmailStmt *sql.Stmt
	NextIDStmt     *sql.Stmt
	UpdateUserStmt *sql.Stmt
}

func NewUserStorage(db *DB) (*UserStorage, error) {
	us := &UserStorage{statementStorage: newStatementsStorage(db)}

	stmts := []stmt{
		{Query: createUserQuery, Dst: &us.CreateStmt},
		{Query: getUserByIDQuery, Dst: &us.GetByIDStmt},
		{Query: getUserByEmailQuery, Dst: &us.GetByEmailStmt},
		{Query: getNextIDQuery, Dst: &us.NextIDStmt},
		{Query: updateUserQuery, Dst: &us.UpdateUserStmt},
	}

	if err := us.initStatements(stmts); err != nil {
		return nil, errors.Wrap(err, "can't init statements in users")
	}

	return us, nil
}

func scanUser(scanner sqlScanner, user *users.User) error {
	err := scanner.Scan(&user.ID, &user.FirstName, &user.LastName, &user.Birthday, &user.Email,
		&user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return err
	}

	return nil
}

const createUserQuery = `INSERT INTO users (id, first_name, last_name, birthday, 
email, password, created_at, updated_at) 
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

func (us *UserStorage) Create(user *users.User) error {
	_, err := us.CreateStmt.Exec(user.ID, user.FirstName, user.LastName, user.Birthday, user.Email,
		user.Password, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return errors.Wrapf(err, "can't create user with bday")
	}

	return nil
}

const getUserByIDQuery = `SELECT id, first_name, last_name, birthday, email, password, created_at, updated_at 
FROM users 
WHERE id = $1`

func (us *UserStorage) GetByID(id int64) (*users.User, error) {
	row := us.GetByIDStmt.QueryRow(id)
	user := users.User{}
	err := scanUser(row, &user)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "can't get user by id")
	}

	return &user, nil
}

const getUserByEmailQuery = `SELECT id, first_name, last_name, birthday, email, password, created_at, updated_at 
FROM users 
WHERE email = $1`

func (us *UserStorage) GetByEmail(email string) (*users.User, error) {
	row := us.GetByEmailStmt.QueryRow(email)
	user := users.User{}
	err := scanUser(row, &user)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "can't get user by email")
	}

	return &user, nil
}

const getNextIDQuery = `SELECT MAX(id) FROM users`

func (us *UserStorage) NextID() (int64, error) {
	row := us.NextIDStmt.QueryRow()

	nextID := sql.NullInt64{}
	err := row.Scan(&nextID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 1, nil
		}
		return 0, errors.Wrapf(err, "can't count next id")
	}

	return nextID.Int64 + 1, nil
}

const updateUserQuery = `UPDATE users SET first_name = $1, last_name = $2, birthday = $3, email = $4, 
password = $5, updated_at = $6 WHERE id = $7`

func (us *UserStorage) UpdateUser(user *users.User) error {

	_, err := us.UpdateUserStmt.Exec(user.FirstName, user.LastName, user.Birthday,
		user.Email, user.Password, time.Now().UTC(), user.ID)
	if err != nil {
		return errors.Wrapf(err, "can't update user")
	}

	return nil
}
