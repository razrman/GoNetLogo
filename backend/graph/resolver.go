package graph

import (
	"fmt"

	"gonetlogo/backend/graph/model"
	"gonetlogo/backend/session"
)

// Resolver holds the process-wide session store.
// This file is not regenerated.
type Resolver struct {
	Store *session.Store
}

func (r *Resolver) session(id string) (*session.Session, error) {
	s, ok := r.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("no session %s", id)
	}
	return s, nil
}

func (r *Resolver) run(id string, fn func(*session.Session) error) (*model.Session, error) {
	s, err := r.session(id)
	if err != nil {
		return nil, err
	}
	if err := fn(s); err != nil {
		return nil, err
	}
	return s.Session(), nil
}
