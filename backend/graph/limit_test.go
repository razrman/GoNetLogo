package graph

import (
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"gonetlogo/backend/session"
)

func TestCreateSessionReportsTheCap(t *testing.T) {
	store := session.NewStore()
	t.Cleanup(store.Close)
	store.SetMaxSessions(1)
	h := handler.New(NewExecutableSchema(Config{Resolvers: &Resolver{Store: store}}))
	h.AddTransport(transport.POST{})
	c := client.New(h)

	var created struct {
		CreateSession struct{ ID string }
	}
	c.MustPost(`mutation ($m: String!) { createSession(model: $m) { id } }`, &created, client.Var("m", "fire.nlogo"))
	if created.CreateSession.ID == "" {
		t.Fatal("missing session id")
	}
	err := c.Post(`mutation ($m: String!) { createSession(model: $m) { id } }`, &created, client.Var("m", "fire.nlogo"))
	if err == nil || !strings.Contains(err.Error(), "too many sessions") {
		t.Fatalf("createSession err = %v", err)
	}
}
