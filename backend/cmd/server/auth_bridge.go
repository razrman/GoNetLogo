package main

import (
	"fmt"
	"sync"
	"time"

	"gonetlogo/backend/session"
	"gonetlogo/internal/auth"
)

// modelBridge shows a signed-in user the model workspaces they opened.
// mu keeps "create the first session" from running twice at once.
type modelBridge struct {
	mu    sync.Mutex
	store *session.Store
}

func bridgeWorkspace(item session.Workspace) auth.ModelWorkspace {
	return auth.ModelWorkspace{
		ID:        item.ID,
		ModelName: item.ModelName,
		Created:   item.Created.UTC().Format(time.RFC3339),
		Status:    item.Status,
	}
}

func (b modelBridge) List(userID string) []auth.ModelWorkspace {
	items := b.store.ForOwner(userID)
	out := make([]auth.ModelWorkspace, 0, len(items))
	for _, item := range items {
		out = append(out, bridgeWorkspace(item))
	}
	return out
}

// Open returns this user's open model session, creating fire.nlogo when
// they have none. The bool is true only when a session was created.
func (b *modelBridge) Open(userID string) (auth.ModelWorkspace, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if userID == "" {
		return auth.ModelWorkspace{}, false, fmt.Errorf("sign in to continue")
	}
	for _, item := range b.store.ForOwner(userID) {
		if item.Status != session.StatusEnding {
			return bridgeWorkspace(item), false, nil
		}
	}
	s, err := b.store.Create("fire.nlogo")
	if err != nil {
		return auth.ModelWorkspace{}, false, err
	}
	s.SetOwner(userID)
	for _, item := range b.store.ForOwner(userID) {
		if item.ID == s.ID() {
			return bridgeWorkspace(item), true, nil
		}
	}
	return auth.ModelWorkspace{ID: s.ID(), ModelName: "fire.nlogo", Status: session.StatusStopped}, true, nil
}

func (b modelBridge) Claim(userID, id string) bool {
	s, ok := b.store.Get(id)
	if !ok || userID == "" {
		return false
	}
	owner := s.OwnerID()
	if owner != "" && owner != userID {
		return false
	}
	s.SetOwner(userID)
	return true
}

func (b modelBridge) End(userID, id string) bool {
	for _, item := range b.store.ForOwner(userID) {
		if item.ID == id {
			return b.store.Release(id)
		}
	}
	return false
}
