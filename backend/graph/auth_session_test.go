package graph

import (
	"context"
	"testing"

	"gonetlogo/backend/session"
	"gonetlogo/internal/auth"
)

func TestSignedInCreateSessionIsListedWithItsID(t *testing.T) {
	st := session.NewStore()
	t.Cleanup(st.Close)
	r := &mutationResolver{&Resolver{Store: st}}

	ctx := auth.WithIdentity(context.Background(), auth.Identity{UserID: "user-1", Email: "a@b.co"})
	created, err := r.CreateSession(ctx, "fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	items := st.ForOwner("user-1")
	if len(items) != 1 || items[0].ID != created.ID || items[0].ModelName != "fire.nlogo" {
		t.Fatalf("owned %#v created %s", items, created.ID)
	}
	if items[0].Status != session.StatusStopped {
		t.Fatalf("before go %#v", items[0])
	}

	live, ok := st.Get(created.ID)
	if !ok {
		t.Fatal("created session is missing")
	}
	if err := live.Setup(); err != nil {
		t.Fatal(err)
	}
	if err := live.Go(); err != nil {
		t.Fatal(err)
	}
	items = st.ForOwner("user-1")
	if len(items) != 1 || items[0].ID != created.ID || items[0].Status != session.StatusRunning {
		t.Fatalf("running %#v", items)
	}

	anon, err := r.CreateSession(context.Background(), "fire.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	other, ok := st.Get(anon.ID)
	if !ok || other.OwnerID() != "" {
		t.Fatal("an anonymous session was given an owner")
	}
	if len(st.ForOwner("user-1")) != 1 {
		t.Fatal("the anonymous session was listed for the signed-in user")
	}
}
