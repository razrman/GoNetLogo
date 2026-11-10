package graph

import (
	"net/http"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/gorilla/websocket"

	"gonetlogo/backend/session"
)

func lifecycleClient(t *testing.T, grace time.Duration) (*session.Store, *client.Client) {
	t.Helper()
	store := session.NewStoreWith(session.Lifecycle{
		Grace: grace,
		Idle:  time.Hour,
		Sweep: time.Hour,
	})
	t.Cleanup(store.Close)
	h := handler.New(NewExecutableSchema(Config{Resolvers: &Resolver{Store: store}}))
	h.AddTransport(transport.POST{})
	h.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	})
	return store, client.New(h)
}

func createFire(t *testing.T, c *client.Client) string {
	t.Helper()
	var resp struct {
		CreateSession struct{ ID string }
	}
	c.MustPost(`mutation ($m: String!) { createSession(model: $m) { id } }`, &resp, client.Var("m", "fire.nlogo"))
	if resp.CreateSession.ID == "" {
		t.Fatal("empty session id")
	}
	return resp.CreateSession.ID
}

func waitStoreGone(t *testing.T, st *session.Store, id string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := st.Get(id); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s still registered", id)
}

func TestWebsocketDisconnectEndsSessionAfterGrace(t *testing.T) {
	grace := 200 * time.Millisecond
	store, c := lifecycleClient(t, grace)
	id := createFire(t, c)
	sub := c.Websocket(`subscription ($id: ID!) { frames(sessionId: $id) { sessionId running } }`, client.Var("id", id))
	var frame struct {
		Frames struct {
			SessionID string
			Running   bool
		}
	}
	if err := sub.Next(&frame); err != nil {
		t.Fatal(err)
	}
	if frame.Frames.SessionID != id {
		t.Fatalf("frame session %s", frame.Frames.SessionID)
	}
	sub.Close()
	if _, ok := store.Get(id); !ok {
		t.Fatal("session ended before the grace period")
	}
	waitStoreGone(t, store, id)
}

func TestWebsocketReconnectKeepsSession(t *testing.T) {
	grace := 250 * time.Millisecond
	store, c := lifecycleClient(t, grace)
	id := createFire(t, c)
	sub := c.Websocket(`subscription ($id: ID!) { frames(sessionId: $id) { sessionId } }`, client.Var("id", id))
	var frame struct {
		Frames struct{ SessionID string }
	}
	if err := sub.Next(&frame); err != nil {
		t.Fatal(err)
	}
	sub.Close()
	again := c.Websocket(`subscription ($id: ID!) { frames(sessionId: $id) { sessionId } }`, client.Var("id", id))
	defer again.Close()
	if err := again.Next(&frame); err != nil {
		t.Fatal(err)
	}
	time.Sleep(grace + 200*time.Millisecond)
	if _, ok := store.Get(id); !ok {
		t.Fatal("reconnect within the grace period lost the session")
	}
}
