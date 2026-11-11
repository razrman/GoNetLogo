package session

import (
	"errors"
	"testing"
	"time"
)

func TestMaxSessionsRejectsAndFreesASlot(t *testing.T) {
	st := NewStoreWith(shortLife(40 * time.Millisecond))
	t.Cleanup(st.Close)
	st.SetMaxSessions(1)

	first, err := st.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create("fire.nlogo"); !errors.Is(err, ErrTooManySessions) {
		t.Fatalf("second create err = %v", err)
	}
	if _, ok := st.Get(first.ID()); !ok {
		t.Fatal("the capped create removed the live session")
	}

	if _, ok := st.Attach(first.ID()); !ok {
		t.Fatal("attach")
	}
	st.Detach(first.ID())
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := st.Get(first.ID()); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session did not leave after the grace period")
		}
		time.Sleep(10 * time.Millisecond)
	}
	second, err := st.Create("fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID() == first.ID() {
		t.Fatal("reused the closed session id")
	}
}

func TestMaxSessionsZeroIsUnlimited(t *testing.T) {
	st := NewStore()
	t.Cleanup(st.Close)
	st.SetMaxSessions(1)
	if _, err := st.Create("fire.nlogo"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create("fire.nlogo"); !errors.Is(err, ErrTooManySessions) {
		t.Fatalf("capped err = %v", err)
	}
	st.SetMaxSessions(0)
	if _, err := st.Create("fire.nlogo"); err != nil {
		t.Fatalf("unlimited err = %v", err)
	}
	st.SetMaxSessions(-3)
	if _, err := st.Create("fire.nlogo"); err != nil {
		t.Fatalf("negative cap err = %v", err)
	}
}
