package auth

import "time"

// User is one account in our store. The password hash is only present
// for the local provider. Roles live here, not in the identity provider.
type User struct {
	ID           string    `json:"id"`
	Provider     string    `json:"provider"`
	Subject      string    `json:"subject"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"displayName"`
	PasswordHash string    `json:"passwordHash,omitempty"`
	Role         string    `json:"role"`
	Disabled     bool      `json:"disabled"`
	Created      time.Time `json:"created"`
	LastLogin    time.Time `json:"lastLogin,omitempty"`
}

// Public drops the password hash.
func (u User) Public() User {
	u.PasswordHash = ""
	return u
}

// UserStore is the account database. The file implementation is a
// stand-in; a later pass can replace it with a database.
type UserStore interface {
	Create(user User) error
	ByEmail(email string) (User, bool, error)
	ByID(id string) (User, bool, error)
	Update(user User) error
	Delete(id string) error
	List() ([]User, error)
	// PromoteEmails marks existing users with these emails as admins.
	PromoteEmails(emails []string) error
}
