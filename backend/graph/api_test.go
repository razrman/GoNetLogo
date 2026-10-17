package graph

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/gorilla/websocket"

	"gonetlogo/backend/session"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	store := session.NewStore()
	t.Cleanup(store.Close)
	h := handler.New(NewExecutableSchema(Config{Resolvers: &Resolver{Store: store}}))
	h.AddTransport(transport.POST{})
	h.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	})
	return client.New(h)
}

func TestModelsIncludeFire(t *testing.T) {
	c := newClient(t)
	var resp struct {
		Models []string
	}
	c.MustPost(`query { models }`, &resp)
	found := false
	for _, name := range resp.Models {
		if name == "fire.nlogo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("models = %v, want fire.nlogo", resp.Models)
	}
}

func TestSessionsAreIsolated(t *testing.T) {
	c := newClient(t)
	a := create(t, c, "fire.nlogo")
	b := create(t, c, "fire.nlogo")
	if a.ID == b.ID {
		t.Fatal("two sessions share an id")
	}
	setup(t, c, a.ID)
	var resp struct {
		Session *struct {
			TicksStarted bool
		}
	}
	c.MustPost(`query ($id: ID!) { session(id: $id) { ticksStarted } }`, &resp, client.Var("id", b.ID))
	if resp.Session == nil {
		t.Fatal("session B missing")
	}
	if resp.Session.TicksStarted {
		t.Fatal("setup on session A started ticks in session B")
	}
}

func TestCommandCenterArithmetic(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")
	var resp struct {
		RunCommand struct {
			Ok     bool
			Output *string
			Error  *string
		}
	}
	c.MustPost(`mutation ($id: ID!) { runCommand(sessionId: $id, text: "2 + 3 * 4") { ok output error } }`, &resp, client.Var("id", s.ID))
	if !resp.RunCommand.Ok {
		t.Fatalf("command failed: %v", resp.RunCommand.Error)
	}
	if resp.RunCommand.Output == nil || *resp.RunCommand.Output != "14" {
		t.Fatalf("output = %v, want 14", resp.RunCommand.Output)
	}
}

func TestSetupStepAndSlider(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")
	if s.TicksStarted {
		t.Fatal("new session already started ticks")
	}
	var slider int
	found := false
	for _, w := range s.Widgets {
		if w.Kind == "slider" && w.Variable != nil && *w.Variable == "density" {
			slider = w.Index
			found = true
		}
	}
	if !found {
		t.Fatal("fire model has no density slider")
	}

	var set struct {
		SetSlider struct {
			Widgets []struct {
				Index int
				Value *float64
			}
		}
	}
	c.MustPost(`mutation ($id: ID!, $i: Int!) {
		setSlider(sessionId: $id, index: $i, value: 100) { widgets { index value } }
	}`, &set, client.Var("id", s.ID), client.Var("i", slider))
	var got *float64
	for _, w := range set.SetSlider.Widgets {
		if w.Index == slider {
			got = w.Value
		}
	}
	if got == nil || *got != 100 {
		t.Fatalf("density = %v, want 100", got)
	}

	started := setup(t, c, s.ID)
	if !started.TicksStarted {
		t.Fatal("setup did not start ticks")
	}
	if started.Ticks == nil || *started.Ticks != 0 {
		t.Fatalf("ticks after setup = %v, want 0", started.Ticks)
	}

	var stepped struct {
		Step struct {
			Ticks *float64
		}
	}
	c.MustPost(`mutation ($id: ID!) { step(sessionId: $id) { ticks } }`, &stepped, client.Var("id", s.ID))
	if stepped.Step.Ticks == nil || *stepped.Step.Ticks <= 0 {
		t.Fatalf("ticks after step = %v, want > 0", stepped.Step.Ticks)
	}
}

func TestFramesSubscription(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "life.nlogo")
	sub := c.Websocket(`subscription ($id: ID!) { frames(sessionId: $id) { sessionId ticksStarted running } }`, client.Var("id", s.ID))
	defer sub.Close()

	var first struct {
		Frames struct {
			SessionID    string
			TicksStarted bool
			Running      bool
		}
	}
	if err := sub.Next(&first); err != nil {
		t.Fatal(err)
	}
	if first.Frames.SessionID != s.ID {
		t.Fatalf("frame session = %s, want %s", first.Frames.SessionID, s.ID)
	}
	if first.Frames.TicksStarted {
		t.Fatal("initial frame already started ticks")
	}

	setup(t, c, s.ID)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := sub.Next(&first); err != nil {
			t.Fatal(err)
		}
		if first.Frames.TicksStarted {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("subscription did not observe setup")
		}
	}
}

func TestSetWorld(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")

	var before struct {
		WorldSettings struct {
			MinPxcor  int
			MaxPxcor  int
			PatchSize float64
			WrapX     bool
			WrapY     bool
		}
	}
	c.MustPost(`query ($id: ID!) {
		worldSettings(sessionId: $id) { minPxcor maxPxcor patchSize wrapX wrapY }
	}`, &before, client.Var("id", s.ID))
	if before.WorldSettings.MinPxcor != -12 || before.WorldSettings.MaxPxcor != 12 || before.WorldSettings.WrapX || before.WorldSettings.WrapY {
		t.Fatalf("initial settings %+v", before.WorldSettings)
	}

	var resized struct {
		SetWorld struct {
			World struct {
				Width       int
				Height      int
				MinPxcor    int
				MaxPxcor    int
				MinPycor    int
				MaxPycor    int
				PatchSize   float64
				WrapX       bool
				WrapY       bool
				PatchColors string
			}
		}
	}
	c.MustPost(`mutation ($id: ID!) {
		setWorld(sessionId: $id, minPxcor: 0, maxPxcor: 99, minPycor: 0, maxPycor: 99, patchSize: 8, wrapX: true, wrapY: false) {
			world { width height minPxcor maxPxcor minPycor maxPycor patchSize wrapX wrapY patchColors }
		}
	}`, &resized, client.Var("id", s.ID))
	w := resized.SetWorld.World
	if w.Width != 100 || w.Height != 100 || w.MinPxcor != 0 || w.MaxPxcor != 99 || w.MinPycor != 0 || w.MaxPycor != 99 {
		t.Fatalf("world size %+v", w)
	}
	if w.PatchSize != 8 || !w.WrapX || w.WrapY {
		t.Fatalf("settings patchSize %v wrap %v %v", w.PatchSize, w.WrapX, w.WrapY)
	}
	raw, err := base64.StdEncoding.DecodeString(w.PatchColors)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 100*100*3 {
		t.Fatalf("patch colors = %d bytes, want %d", len(raw), 100*100*3)
	}

	var cmd struct {
		RunCommand struct {
			Ok     bool
			Output *string
			Error  *string
		}
	}
	c.MustPost(`mutation ($id: ID!) { runCommand(sessionId: $id, text: "crt 20") { ok output error } }`, &cmd, client.Var("id", s.ID))
	if !cmd.RunCommand.Ok {
		t.Fatalf("crt: %v", cmd.RunCommand.Error)
	}
	var after struct {
		Session struct {
			World struct {
				PatchColors string
				Turtles     []struct{ Who int }
			}
		}
	}
	c.MustPost(`query ($id: ID!) {
		session(id: $id) { world { patchColors turtles { who } } }
	}`, &after, client.Var("id", s.ID))
	if len(after.Session.World.Turtles) != 20 {
		t.Fatalf("turtles = %d, want 20", len(after.Session.World.Turtles))
	}
	// A second resize kills the turtles and still streams the full grid.
	c.MustPost(`mutation ($id: ID!) {
		setWorld(sessionId: $id, minPxcor: 0, maxPxcor: 99, minPycor: 0, maxPycor: 99, patchSize: 8, wrapX: true, wrapY: false) {
			world { width }
		}
	}`, &resized, client.Var("id", s.ID))
	c.MustPost(`query ($id: ID!) { session(id: $id) { world { turtles { who } } } }`, &after, client.Var("id", s.ID))
	if len(after.Session.World.Turtles) != 0 {
		t.Fatalf("turtles survived resize: %d", len(after.Session.World.Turtles))
	}

	var rejected struct {
		SetWorld struct{ ID string }
	}
	err = c.Post(`mutation ($id: ID!) {
		setWorld(sessionId: $id, minPxcor: 1, maxPxcor: 10, minPycor: 0, maxPycor: 10, patchSize: 8, wrapX: false, wrapY: false) { id }
	}`, &rejected, client.Var("id", s.ID))
	if err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("expected origin error, got %v", err)
	}
}

type created struct {
	ID           string
	Ticks        *float64
	TicksStarted bool
	Widgets      []struct {
		Index    int
		Kind     string
		Variable *string
	}
}

func create(t *testing.T, c *client.Client, modelName string) created {
	t.Helper()
	var resp struct {
		CreateSession created
	}
	c.MustPost(`mutation ($m: String!) {
		createSession(model: $m) { id ticksStarted widgets { index kind variable } }
	}`, &resp, client.Var("m", modelName))
	if resp.CreateSession.ID == "" {
		t.Fatal("empty session id")
	}
	return resp.CreateSession
}

func setup(t *testing.T, c *client.Client, id string) created {
	t.Helper()
	var resp struct {
		Setup created
	}
	c.MustPost(`mutation ($id: ID!) { setup(sessionId: $id) { id ticksStarted ticks widgets { index kind variable } } }`, &resp, client.Var("id", id))
	return resp.Setup
}
