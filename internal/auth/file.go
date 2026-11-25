package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileStore is a JSON user list. Writes replace the file atomically.
// The process holds one mutex so two sign-ups cannot clobber each other.
// The file is ordinary disk: a container that uses a temporary directory
// loses it when the container is replaced.
type FileStore struct {
	mu   sync.Mutex
	path string
}

type fileBody struct {
	Users []User `json:"users"`
}

// OpenFileStore opens or creates the user file.
func OpenFileStore(path string) (*FileStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("users file path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	st := &FileStore{path: path}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := st.write(fileBody{Users: []User{}}); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if _, err := st.read(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *FileStore) Create(user User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := s.read()
	if err != nil {
		return err
	}
	for _, existing := range body.Users {
		if strings.EqualFold(existing.Email, user.Email) {
			return fmt.Errorf("an account with that email already exists")
		}
	}
	if user.Created.IsZero() {
		user.Created = time.Now().UTC()
	}
	body.Users = append(body.Users, user)
	return s.write(body)
}

func (s *FileStore) ByEmail(email string) (User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := s.read()
	if err != nil {
		return User{}, false, err
	}
	for _, user := range body.Users {
		if strings.EqualFold(user.Email, email) {
			return user, true, nil
		}
	}
	return User{}, false, nil
}

func (s *FileStore) ByID(id string) (User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := s.read()
	if err != nil {
		return User{}, false, err
	}
	for _, user := range body.Users {
		if user.ID == id {
			return user, true, nil
		}
	}
	return User{}, false, nil
}

func (s *FileStore) Update(user User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := s.read()
	if err != nil {
		return err
	}
	for i := range body.Users {
		if body.Users[i].ID == user.ID {
			body.Users[i] = user
			return s.write(body)
		}
	}
	return fmt.Errorf("no user %s", user.ID)
}

func (s *FileStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := s.read()
	if err != nil {
		return err
	}
	next := body.Users[:0]
	found := false
	for _, user := range body.Users {
		if user.ID == id {
			found = true
			continue
		}
		next = append(next, user)
	}
	if !found {
		return fmt.Errorf("no user %s", id)
	}
	body.Users = next
	return s.write(body)
}

func (s *FileStore) List() ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := s.read()
	if err != nil {
		return nil, err
	}
	out := append([]User(nil), body.Users...)
	return out, nil
}

func (s *FileStore) PromoteEmails(emails []string) error {
	want := map[string]bool{}
	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			want[email] = true
		}
	}
	if len(want) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := s.read()
	if err != nil {
		return err
	}
	changed := false
	for i := range body.Users {
		if want[strings.ToLower(body.Users[i].Email)] && body.Users[i].Role != RoleAdmin {
			body.Users[i].Role = RoleAdmin
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.write(body)
}

func (s *FileStore) read() (fileBody, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return fileBody{}, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return fileBody{Users: []User{}}, nil
	}
	var body fileBody
	if err := json.Unmarshal(b, &body); err != nil {
		return fileBody{}, fmt.Errorf("users file %s: %w", s.path, err)
	}
	if body.Users == nil {
		body.Users = []User{}
	}
	return body, nil
}

func (s *FileStore) write(body fileBody) error {
	if body.Users == nil {
		body.Users = []User{}
	}
	b, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
