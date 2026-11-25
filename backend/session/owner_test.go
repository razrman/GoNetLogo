package session

import (
	"testing"
	"time"
)

func TestForOwnerStatusFollowsTheSession(t *testing.T) {
	st := NewStoreWith(Lifecycle{
		Grace:     time.Hour,
		Idle:      time.Hour,
		Sweep:     time.Hour,
		Heartbeat: time.Hour,
	})
	t.Cleanup(st.Close)

	s, err := st.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	s.SetOwner("user-1")
	other, err := st.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	other.SetOwner("user-2")

	got := oneWorkspace(t, st, "user-1")
	if got.ID != s.ID() || got.ModelName == "" || got.Status != StatusStopped || got.Running {
		t.Fatalf("fresh session %#v", got)
	}

	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	if err := s.Go(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !s.Running() {
		if time.Now().After(deadline) {
			t.Fatal("go did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	got = oneWorkspace(t, st, "user-1")
	if got.Status != StatusRunning || !got.Running {
		t.Fatalf("running %#v", got)
	}

	s.Stop()
	got = oneWorkspace(t, st, "user-1")
	if got.Status != StatusStopped || got.Running {
		t.Fatalf("stopped %#v", got)
	}

	if !st.Release(s.ID()) {
		t.Fatal("release")
	}
	got = oneWorkspace(t, st, "user-1")
	if got.Status != StatusEnding || got.Running {
		t.Fatalf("ending %#v", got)
	}
	if len(st.ForOwner("user-2")) != 1 || st.ForOwner("user-2")[0].ID != other.ID() {
		t.Fatal("the other user's workspace changed")
	}
}

func oneWorkspace(t *testing.T, st *Store, userID string) Workspace {
	t.Helper()
	items := st.ForOwner(userID)
	if len(items) != 1 {
		t.Fatalf("workspaces %#v", items)
	}
	return items[0]
}
