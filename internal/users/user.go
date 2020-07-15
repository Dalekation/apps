package user

import (
	"encoding/json"
	"strconv"
	"time"
)

type User struct {
	ID        int64      `json:"-"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Birthday  *time.Time `json:"birthday,omitempty"`
	Email     string     `json:"email"`
	Password  string     `json:"-"`
	CreatedAt time.Time  `json:"-"`
	UpdatedAt time.Time  `json:"-"`
}

type Storage interface {
	Create(user *User) error
	GetByID(id int64) (*User, error)
	GetByEmail(email string) (*User, error)
	NextID() (int64, error)
	UpdateUser(user *User) error
}

func (user *User) MarshalJSON() ([]byte, error) {
	value := map[string]string{
		"user_id":    strconv.FormatInt(user.ID, 10),
		"first_name": user.FirstName,
		"last_name":  user.LastName,
		"email":      user.Email,
	}
	if user.Birthday != nil {
		value["birthday"] = user.Birthday.String()[:10]
	}

	return json.Marshal(value)
}

func (user *User) UnmarshalJSON(data []byte) error {
	request := map[string]string{}
	err := json.Unmarshal(data, &request)
	if err != nil {
		return err
	}

	if _, ok := request["first_name"]; ok {
		user.FirstName = request["first_name"]
	}

	if _, ok := request["last_name"]; ok {
		user.LastName = request["last_name"]
	}

	if _, ok := request["email"]; ok {
		user.Email = request["email"]
	}

	if _, ok := request["password"]; ok {
		user.Password = request["password"]
	}

	if _, ok := request["birthday"]; ok {
		birthday, err := time.Parse(time.RFC3339, request["birthday"]+"T00:00:00Z")
		if err != nil {
			return err
		}
		user.Birthday = &birthday
	}

	return nil
}
