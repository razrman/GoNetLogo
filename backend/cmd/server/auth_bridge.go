package main

import (
	"time"

	"gonetlogo/backend/session"
	"gonetlogo/internal/auth"
)

// modelBridge shows a signed-in user the model workspaces they opened.
type modelBridge struct {
	store *session.Store
}

func (b modelBridge) List(userID string) []auth.ModelWorkspace {
	items := b.store.ForOwner(userID)
	out := make([]auth.ModelWorkspace, 0, len(items))
	for _, item := range items {
		out = append(out, auth.ModelWorkspace{
			ID:        item.ID,
			ModelName: item.ModelName,
			Created:   item.Created.UTC().Format(time.RFC3339),
			Running:   item.Running,
		})
	}
	return out
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
