package graph

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/gorilla/websocket"

	"gonetlogo/backend/session"
	"gonetlogo/pkg/nl"
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

func TestLibraryStartsInLibraryFolder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MODELS_DIR", dir)
	t.Setenv("GONETLOGO_MODELS_DIR", "")
	sample := filepath.Join(dir, "library", "Sample Models")
	if err := os.MkdirAll(sample, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sample, "Fire.nlogox"), []byte("<model></model>"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newClient(t)
	var listed struct {
		Library struct {
			Path      string
			Parent    *string
			Available bool
			Entries   []struct {
				Name      string
				Path      string
				Directory bool
			}
		}
	}
	c.MustPost(`query { library { path parent available entries { name path directory } } }`, &listed)
	if !listed.Library.Available || listed.Library.Path != "library" {
		t.Fatalf("library %+v", listed.Library)
	}
	if listed.Library.Parent == nil || *listed.Library.Parent != "" {
		t.Fatalf("parent %#v", listed.Library.Parent)
	}
	if len(listed.Library.Entries) != 1 || listed.Library.Entries[0].Name != "Sample Models" || !listed.Library.Entries[0].Directory {
		t.Fatalf("entries %+v", listed.Library.Entries)
	}
	if listed.Library.Entries[0].Path != "library/Sample Models" {
		t.Fatalf("entry path %q", listed.Library.Entries[0].Path)
	}

	var rootList struct {
		Library struct {
			Path    string
			Parent  *string
			Entries []struct{ Name string }
		}
	}
	c.MustPost(`query($path: String) { library(path: $path) { path parent entries { name } } }`, &rootList, client.Var("path", ""))
	if rootList.Library.Path != "" || rootList.Library.Parent != nil {
		t.Fatalf("root %+v", rootList.Library)
	}
	found := false
	for _, e := range rootList.Library.Entries {
		if e.Name == "library" {
			found = true
		}
	}
	if !found {
		t.Fatalf("root entries %+v", rootList.Library.Entries)
	}
}

func TestRobbyIsListedAndLoads(t *testing.T) {
	c := newClient(t)
	var resp struct {
		Models []string
	}
	c.MustPost(`query { models }`, &resp)
	found := false
	for _, name := range resp.Models {
		if name == "Robby the Robot.nlogo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("models = %v, want Robby the Robot.nlogo", resp.Models)
	}
	var created struct {
		CreateSession struct{ ID, ModelName string }
	}
	c.MustPost(`mutation ($m: String!) {
		createSession(model: $m) { id modelName }
	}`, &created, client.Var("m", "Robby the Robot.nlogo"))
	if created.CreateSession.ModelName != "Robby the Robot.nlogo" || created.CreateSession.ID == "" {
		t.Fatalf("createSession %+v", created.CreateSession)
	}
	t.Setenv("MODELS_DIR", filepath.Join("..", "..", "models"))
	var fromFile struct {
		LoadModelFile struct{ ModelName string }
	}
	c.MustPost(`mutation ($id: ID!, $path: String!) {
		loadModelFile(sessionId: $id, path: $path) { modelName }
	}`, &fromFile, client.Var("id", created.CreateSession.ID), client.Var("path", "Robby the Robot.nlogo"))
	if fromFile.LoadModelFile.ModelName != "Robby the Robot.nlogo" {
		t.Fatalf("loadModelFile %+v", fromFile.LoadModelFile)
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

func TestRunReadout(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "wolfsheep.nlogo")
	var q struct {
		Session struct {
			Tps     float64
			Timer   string
			Running bool
		}
	}
	query := `query ($id: ID!) { session(id: $id) { tps timer running } }`
	c.MustPost(query, &q, client.Var("id", s.ID))
	if q.Session.Tps != 0 || q.Session.Timer != "00:00.0" || q.Session.Running {
		t.Fatalf("initial %+v", q.Session)
	}

	setup(t, c, s.ID)
	c.MustPost(query, &q, client.Var("id", s.ID))
	if q.Session.Tps != 0 || q.Session.Timer != "00:00.0" {
		t.Fatalf("after setup %+v", q.Session)
	}

	var speed struct{ SetSpeed struct{ ID string } }
	c.MustPost(`mutation ($id: ID!) { setSpeed(sessionId: $id, speed: 100) { id } }`, &speed, client.Var("id", s.ID))
	var goResp struct{ Go struct{ ID string } }
	c.MustPost(`mutation ($id: ID!) { go(sessionId: $id) { id } }`, &goResp, client.Var("id", s.ID))
	time.Sleep(700 * time.Millisecond)
	c.MustPost(query, &q, client.Var("id", s.ID))
	if !q.Session.Running || q.Session.Timer == "00:00.0" || q.Session.Tps <= 0 {
		t.Fatalf("while running %+v", q.Session)
	}

	sub := c.Websocket(`subscription ($id: ID!) { frames(sessionId: $id) { tps timer running } }`, client.Var("id", s.ID))
	defer sub.Close()
	var frame struct {
		Frames struct {
			Tps     float64
			Timer   string
			Running bool
		}
	}
	if err := sub.Next(&frame); err != nil {
		t.Fatal(err)
	}
	if !frame.Frames.Running || frame.Frames.Timer == "" {
		t.Fatalf("frame %+v", frame.Frames)
	}

	var stop struct{ Stop struct{ ID string } }
	c.MustPost(`mutation ($id: ID!) { stop(sessionId: $id) { id } }`, &stop, client.Var("id", s.ID))
	paused := ""
	c.MustPost(query, &q, client.Var("id", s.ID))
	paused = q.Session.Timer
	if q.Session.Running || q.Session.Tps != 0 || paused == "00:00.0" {
		t.Fatalf("stopped %+v", q.Session)
	}
	time.Sleep(400 * time.Millisecond)
	c.MustPost(query, &q, client.Var("id", s.ID))
	if q.Session.Timer != paused || q.Session.Tps != 0 {
		t.Fatalf("paused timer moved from %s to %+v", paused, q.Session)
	}

	commandWhenIdle(t, c, s.ID, "reset-ticks")
	c.MustPost(query, &q, client.Var("id", s.ID))
	if q.Session.Timer != "00:00.0" {
		t.Fatalf("after reset-ticks %+v", q.Session)
	}

	c.MustPost(`mutation ($id: ID!) { go(sessionId: $id) { id } }`, &goResp, client.Var("id", s.ID))
	time.Sleep(400 * time.Millisecond)
	c.MustPost(`mutation ($id: ID!) { stop(sessionId: $id) { id } }`, &stop, client.Var("id", s.ID))
	c.MustPost(query, &q, client.Var("id", s.ID))
	if q.Session.Timer == "00:00.0" {
		t.Fatal("timer stayed at zero while go ran")
	}
	commandWhenIdle(t, c, s.ID, "clear-all")
	c.MustPost(query, &q, client.Var("id", s.ID))
	if q.Session.Timer != "00:00.0" || q.Session.Tps != 0 {
		t.Fatalf("after clear-all %+v", q.Session)
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

// TestSetWorldZBounds reads and sets min-pzcor and max-pzcor on a 3D world.
// A 2D world keeps the page's old fields: Z arguments do not add a Z axis.
func TestSetWorldZBounds(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")

	type bounds struct {
		MinPxcor, MaxPxcor  int
		MinPycor, MaxPycor  int
		MinPzcor, MaxPzcor  int
		Depth               int
		WrapX, WrapY, WrapZ bool
		ThreeD              bool
		PatchSize           float64
	}
	var flat struct{ WorldSettings bounds }
	c.MustPost(`query ($id: ID!) {
		worldSettings(sessionId: $id) {
			minPxcor maxPxcor minPycor maxPycor minPzcor maxPzcor wrapX wrapY wrapZ threeD
		}
	}`, &flat, client.Var("id", s.ID))
	if flat.WorldSettings.ThreeD || flat.WorldSettings.WrapZ || flat.WorldSettings.MinPzcor != 0 || flat.WorldSettings.MaxPzcor != 0 {
		t.Fatalf("2D settings %+v", flat.WorldSettings)
	}

	var ignored struct {
		SetWorld struct{ World bounds }
	}
	c.MustPost(`mutation ($id: ID!) {
		setWorld(sessionId: $id, minPxcor: -5, maxPxcor: 5, minPycor: -5, maxPycor: 5, patchSize: 8, wrapX: false, wrapY: true, minPzcor: -3, maxPzcor: 3, wrapZ: true) {
			world { minPxcor maxPxcor minPycor maxPycor minPzcor maxPzcor depth wrapX wrapY wrapZ threeD patchSize }
		}
	}`, &ignored, client.Var("id", s.ID))
	if got := ignored.SetWorld.World; got.ThreeD || got.WrapZ || got.MinPzcor != 0 || got.MaxPzcor != 0 || got.Depth != 1 || got.MinPxcor != -5 || got.WrapX || !got.WrapY {
		t.Fatalf("2D setWorld changed Z %+v", got)
	}

	src := `<?xml version="1.0"?>
<model version="NetLogo 3D 7.0.0">
  <code><![CDATA[to setup
  clear-all
end
]]></code>
  <widgets>
    <view3d minPxcor="-2" maxPxcor="2" minPycor="-2" maxPycor="2" minPzcor="-1" maxPzcor="4" patchSize="9" wrappingAllowedX="true" wrappingAllowedY="true" wrappingAllowedZ="true"/>
  </widgets>
</model>`
	c.MustPost(`mutation ($id: ID!, $name: String!, $src: String!) {
		loadModelSource(sessionId: $id, filename: $name, source: $src) { id }
	}`, &struct{ LoadModelSource struct{ ID string } }{}, client.Var("id", s.ID), client.Var("name", "box.nlogox3d"), client.Var("src", src))

	var loaded struct{ WorldSettings bounds }
	c.MustPost(`query ($id: ID!) {
		worldSettings(sessionId: $id) {
			minPxcor maxPxcor minPycor maxPycor minPzcor maxPzcor wrapX wrapY wrapZ threeD
		}
	}`, &loaded, client.Var("id", s.ID))
	if got := loaded.WorldSettings; !got.ThreeD || !got.WrapZ || got.MinPzcor != -1 || got.MaxPzcor != 4 || got.MinPxcor != -2 {
		t.Fatalf("3D settings %+v", got)
	}

	err := c.Post(`mutation ($id: ID!) {
		setWorld(sessionId: $id, minPxcor: -2, maxPxcor: 2, minPycor: -2, maxPycor: 2, patchSize: 9, wrapX: true, wrapY: true, minPzcor: -4) { id }
	}`, &struct{ SetWorld struct{ ID string } }{}, client.Var("id", s.ID))
	if err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("expected both Z bounds, got %v", err)
	}
	err = c.Post(`mutation ($id: ID!) {
		setWorld(sessionId: $id, minPxcor: -2, maxPxcor: 2, minPycor: -2, maxPycor: 2, patchSize: 9, wrapX: true, wrapY: true, minPzcor: 1, maxPzcor: 4, wrapZ: true) { id }
	}`, &struct{ SetWorld struct{ ID string } }{}, client.Var("id", s.ID))
	if err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("expected origin error, got %v", err)
	}

	var resized struct {
		SetWorld struct{ World bounds }
	}
	c.MustPost(`mutation ($id: ID!) {
		setWorld(sessionId: $id, minPxcor: -1, maxPxcor: 1, minPycor: -1, maxPycor: 1, minPzcor: -4, maxPzcor: 2, patchSize: 10, wrapX: false, wrapY: false, wrapZ: false) {
			world { minPxcor maxPxcor minPycor maxPycor minPzcor maxPzcor depth wrapX wrapY wrapZ threeD patchSize }
		}
	}`, &resized, client.Var("id", s.ID))
	if got := resized.SetWorld.World; got.MinPzcor != -4 || got.MaxPzcor != 2 || got.Depth != 7 || got.WrapZ || !got.WrapX || !got.WrapY || !got.ThreeD || got.PatchSize != 10 || got.MinPxcor != -1 || got.MaxPxcor != 1 {
		t.Fatalf("resized 3D world %+v", got)
	}

	// Omitting Z leaves the axis alone, including wrapping that was turned off.
	c.MustPost(`mutation ($id: ID!) {
		setWorld(sessionId: $id, minPxcor: -1, maxPxcor: 3, minPycor: -1, maxPycor: 1, patchSize: 10, wrapX: true, wrapY: true) {
			world { maxPxcor minPzcor maxPzcor wrapZ threeD depth }
		}
	}`, &resized, client.Var("id", s.ID))
	if got := resized.SetWorld.World; got.MaxPxcor != 3 || got.MinPzcor != -4 || got.MaxPzcor != 2 || got.WrapZ || !got.ThreeD || got.Depth != 7 {
		t.Fatalf("Z changed when omitted %+v", got)
	}

	c.MustPost(`query ($id: ID!) {
		worldSettings(sessionId: $id) { minPzcor maxPzcor wrapZ threeD }
	}`, &loaded, client.Var("id", s.ID))
	if got := loaded.WorldSettings; got.MinPzcor != -4 || got.MaxPzcor != 2 || got.WrapZ || !got.ThreeD {
		t.Fatalf("settings after resize %+v", got)
	}
}

func TestLoadModelSourceAndFile(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")

	nlogo := "globals [pace]\nto setup\n  clear-all\n  reset-ticks\nend\nto go\n  tick\nend\n@#$#@#$#@\nGRAPHICS-WINDOW\n0\n0\n0\n0\n0\n0\n12\n0\n0\n0\n0\n0\n0\n1\n1\n0\n-1\n1\n-1\n1\n\n@#$#@#$#@\n"
	nlogox := `<?xml version="1.0"?>
<model version="NetLogo 7.0.0">
  <code><![CDATA[globals [pace]
to setup
  clear-all
  reset-ticks
end
]]></code>
  <widgets>
    <view minPxcor="-2" maxPxcor="2" minPycor="-2" maxPycor="2" patchSize="9" wrappingAllowedX="true" wrappingAllowedY="false"/>
    <slider variable="pace" min="0" max="5" default="2" step="1"/>
  </widgets>
</model>`

	var loaded struct {
		LoadModelSource struct {
			ModelName string
			Code      string
			World     struct {
				MinPxcor int
				MaxPxcor int
			}
			Widgets []struct {
				Kind     string
				Variable *string
			}
		}
	}
	c.MustPost(`mutation ($id: ID!, $name: String!, $src: String!) {
		loadModelSource(sessionId: $id, filename: $name, source: $src) {
			modelName
			code
			world { minPxcor maxPxcor }
			widgets { kind variable }
		}
	}`, &loaded, client.Var("id", s.ID), client.Var("name", "pace.nlogo"), client.Var("src", nlogo))
	if loaded.LoadModelSource.ModelName != "pace.nlogo" || loaded.LoadModelSource.World.MaxPxcor != 1 {
		t.Fatalf("nlogo %+v", loaded.LoadModelSource)
	}
	if !strings.Contains(loaded.LoadModelSource.Code, "reset-ticks") {
		t.Fatalf("code %q", loaded.LoadModelSource.Code)
	}

	c.MustPost(`mutation ($id: ID!, $name: String!, $src: String!) {
		loadModelSource(sessionId: $id, filename: $name, source: $src) {
			modelName
			code
			world { minPxcor maxPxcor }
			widgets { kind variable }
		}
	}`, &loaded, client.Var("id", s.ID), client.Var("name", "pace.nlogox"), client.Var("src", nlogox))
	if loaded.LoadModelSource.ModelName != "pace.nlogox" || loaded.LoadModelSource.World.MinPxcor != -2 || loaded.LoadModelSource.World.MaxPxcor != 2 {
		t.Fatalf("nlogox %+v", loaded.LoadModelSource)
	}
	foundPace := false
	for _, w := range loaded.LoadModelSource.Widgets {
		if w.Kind == "slider" && w.Variable != nil && *w.Variable == "pace" {
			foundPace = true
		}
	}
	if !foundPace {
		t.Fatal("nlogox source did not install the pace slider")
	}

	err := c.Post(`mutation ($id: ID!, $name: String!, $src: String!) {
		loadModelSource(sessionId: $id, filename: $name, source: $src) { modelName }
	}`, &struct {
		LoadModelSource struct{ ModelName string }
	}{}, client.Var("id", s.ID), client.Var("name", "broken.nlogo"), client.Var("src", "to setup\n  clear-all\nend\nfd 1\n"))
	if err == nil || !strings.Contains(err.Error(), "line 4:") {
		t.Fatalf("compile error %v, want line 4", err)
	}
	var still struct {
		Session struct{ ModelName string }
	}
	c.MustPost(`query ($id: ID!) { session(id: $id) { modelName } }`, &still, client.Var("id", s.ID))
	if still.Session.ModelName != "pace.nlogox" {
		t.Fatalf("failed compile replaced %q", still.Session.ModelName)
	}

	dir := t.TempDir()
	t.Setenv("MODELS_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "pace.nlogo"), []byte(nlogo), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.nlogo")
	if err := os.WriteFile(outside, []byte(nlogo), 0o644); err != nil {
		t.Fatal(err)
	}
	var fromFile struct {
		LoadModelFile struct{ ModelName string }
	}
	c.MustPost(`mutation ($id: ID!, $path: String!) {
		loadModelFile(sessionId: $id, path: $path) { modelName }
	}`, &fromFile, client.Var("id", s.ID), client.Var("path", "pace.nlogo"))
	if fromFile.LoadModelFile.ModelName != "pace.nlogo" {
		t.Fatalf("file load %+v", fromFile.LoadModelFile)
	}
	for _, path := range []string{"../secret.nlogo", `..\secret.nlogo`, outside, "/etc/passwd", "nested/../../pace.nlogo"} {
		err := c.Post(`mutation ($id: ID!, $path: String!) {
			loadModelFile(sessionId: $id, path: $path) { modelName }
		}`, &fromFile, client.Var("id", s.ID), client.Var("path", path))
		if err == nil {
			t.Errorf("loadModelFile accepted %q", path)
		}
	}
	c.MustPost(`query ($id: ID!) { session(id: $id) { modelName } }`, &still, client.Var("id", s.ID))
	if still.Session.ModelName != "pace.nlogo" {
		t.Fatalf("rejected path replaced the model with %q", still.Session.ModelName)
	}

	err = c.Post(`mutation ($id: ID!, $model: String!) {
		loadModel(sessionId: $id, model: $model) { modelName }
	}`, &struct {
		LoadModel struct{ ModelName string }
	}{}, client.Var("id", s.ID), client.Var("model", "../fire.nlogo"))
	if err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("loadModel traversal: %v", err)
	}
}

// TestLoad3DModelProjectsTopDown loads a .nlogox3d source and checks that
// the frame reports the depth and projects the highest colored patch of
// each column, while a 2D world keeps depth 1.
func TestLoad3DModelProjectsTopDown(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")
	src := `<?xml version="1.0"?>
<model version="NetLogo 3D 7.0.0">
  <code><![CDATA[to setup
  clear-all
  ask patch 0 0 -1 [ set pcolor red ]
  ask patch 0 0 1 [ set pcolor blue ]
  ask patch 1 1 -1 [ set pcolor green ]
  crt 2 [ setxyz 0 0 who ]
end
]]></code>
  <widgets>
    <view3d minPxcor="-1" maxPxcor="1" minPycor="-1" maxPycor="1" minPzcor="-1" maxPzcor="1" patchSize="9" wrappingAllowedX="true" wrappingAllowedY="true" wrappingAllowedZ="true"/>
  </widgets>
</model>`
	type world struct {
		Width, Height, Depth int
		PatchColors          string
		Turtles              []struct{ Who int }
	}
	var loaded struct {
		LoadModelSource struct{ World world }
	}
	c.MustPost(`mutation ($id: ID!, $name: String!, $src: String!) {
		loadModelSource(sessionId: $id, filename: $name, source: $src) { world { width height depth } }
	}`, &loaded, client.Var("id", s.ID), client.Var("name", "cube.nlogox3d"), client.Var("src", src))
	if w := loaded.LoadModelSource.World; w.Width != 3 || w.Height != 3 || w.Depth != 3 {
		t.Fatalf("3D world %+v", w)
	}
	commandWhenIdle(t, c, s.ID, "setup")
	var q struct {
		Session struct{ World world }
	}
	c.MustPost(`query ($id: ID!) { session(id: $id) { world { width height depth patchColors turtles { who } } } }`, &q, client.Var("id", s.ID))
	raw, err := base64.StdEncoding.DecodeString(q.Session.World.PatchColors)
	if err != nil || len(raw) != 3*3*3 {
		t.Fatalf("patch colors %d bytes, err %v", len(raw), err)
	}
	pixel := func(px, py int) [3]byte {
		i := ((1-py)*3 + (px + 1)) * 3
		return [3]byte{raw[i], raw[i+1], raw[i+2]}
	}
	blue, green := nl.ColorRGBA(105), nl.ColorRGBA(55)
	if got := pixel(0, 0); got != [3]byte{blue.R, blue.G, blue.B} {
		t.Fatalf("center column %v, want blue (the highest patch)", got)
	}
	if got := pixel(1, 1); got != [3]byte{green.R, green.G, green.B} {
		t.Fatalf("corner column %v, want green", got)
	}
	if got := pixel(-1, -1); got != [3]byte{0, 0, 0} {
		t.Fatalf("empty column %v, want black", got)
	}
	// Higher turtles come later so they paint on top.
	ts := q.Session.World.Turtles
	if len(ts) != 2 || ts[0].Who != 0 || ts[1].Who != 1 {
		t.Fatalf("turtle order %+v", ts)
	}
	commandWhenIdle(t, c, s.ID, "ask turtle 0 [ set zcor 1 ] ask turtle 1 [ set zcor -1 ]")
	c.MustPost(`query ($id: ID!) { session(id: $id) { world { turtles { who } } } }`, &q, client.Var("id", s.ID))
	if ts := q.Session.World.Turtles; len(ts) != 2 || ts[0].Who != 1 {
		t.Fatalf("turtle order after moving %+v", ts)
	}

	s2 := create(t, c, "fire.nlogo")
	c.MustPost(`query ($id: ID!) { session(id: $id) { world { depth } } }`, &q, client.Var("id", s2.ID))
	if q.Session.World.Depth != 1 {
		t.Fatalf("2D depth %d", q.Session.World.Depth)
	}
}

// TestFrame3DFields checks the data the 3D view draws from: packed
// voxels, turtle z and orientation, link z, and the observer pose.
func TestFrame3DFields(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")
	src := `<?xml version="1.0"?>
<model version="NetLogo 3D 7.0.0">
  <code><![CDATA[to setup
  clear-all
  ask patch 1 -1 2 [ set pcolor red ]
  ask patch -2 0 -1 [ set pcolor rgb 1 2 3 ]
  crt 2 [ setxyz who 0 (who * 2 - 1) set heading 90 set pitch 30 set roll 45 ]
  ask turtle 0 [ create-link-with turtle 1 ]
end
]]></code>
  <widgets>
    <view3d minPxcor="-2" maxPxcor="2" minPycor="-2" maxPycor="2" minPzcor="-1" maxPzcor="3" patchSize="9" wrappingAllowedX="true" wrappingAllowedY="true" wrappingAllowedZ="true"/>
  </widgets>
</model>`
	c.MustPost(`mutation ($id: ID!, $name: String!, $src: String!) {
		loadModelSource(sessionId: $id, filename: $name, source: $src) { modelName }
	}`, &struct{ LoadModelSource struct{ ModelName string } }{}, client.Var("id", s.ID), client.Var("name", "frame.nlogox3d"), client.Var("src", src))
	commandWhenIdle(t, c, s.ID, "setup")

	type observer struct {
		X, Y, Z                   float64
		TargetX, TargetY, TargetZ float64
		Revision                  int
	}
	type world struct {
		Depth, MinPzcor, MaxPzcor int
		WrapZ                     bool
		Voxels                    string
		Observer                  *observer
		Turtles                   []struct {
			Who                           int
			X, Y, Z, Heading, Pitch, Roll float64
		}
		Links []struct{ X1, Y1, Z1, X2, Y2, Z2 float64 }
	}
	var q struct {
		Session struct{ World world }
	}
	const query = `query ($id: ID!) { session(id: $id) { world {
		depth minPzcor maxPzcor wrapZ voxels
		observer { x y z targetX targetY targetZ revision }
		turtles { who x y z heading pitch roll }
		links { x1 y1 z1 x2 y2 z2 }
	} } }`
	c.MustPost(query, &q, client.Var("id", s.ID))
	w := q.Session.World
	if w.Depth != 5 || w.MinPzcor != -1 || w.MaxPzcor != 3 || !w.WrapZ {
		t.Fatalf("3D bounds %+v", w)
	}
	raw, err := base64.StdEncoding.DecodeString(w.Voxels)
	const voxelStride = 10
	if err != nil || len(raw) != 2*voxelStride {
		t.Fatalf("voxels %d bytes, err %v", len(raw), err)
	}
	type voxel struct{ x, y, z, r, g, b, a int }
	read := func(i int) voxel {
		v := raw[i*voxelStride:]
		i16 := func(o int) int { return int(int16(uint16(v[o]) | uint16(v[o+1])<<8)) }
		return voxel{i16(0), i16(2), i16(4), int(v[6]), int(v[7]), int(v[8]), int(v[9])}
	}
	red := nl.ColorRGBA(15)
	// Highest layer first. A number color and an RGB list are both opaque.
	if got := read(0); got != (voxel{1, -1, 2, int(red.R), int(red.G), int(red.B), 255}) {
		t.Fatalf("voxel 0 %+v", got)
	}
	if got := read(1); got != (voxel{-2, 0, -1, 1, 2, 3, 255}) {
		t.Fatalf("voxel 1 %+v (negative coordinates and RGB colors)", got)
	}
	if len(w.Turtles) != 2 {
		t.Fatalf("turtles %+v", w.Turtles)
	}
	for _, tt := range w.Turtles {
		if tt.Z != float64(tt.Who*2-1) || tt.Heading != 90 || tt.Pitch != 30 || tt.Roll != 45 {
			t.Fatalf("turtle %+v", tt)
		}
	}
	if len(w.Links) != 1 || w.Links[0].Z1 != -1 || w.Links[0].Z2 != 1 || w.Links[0].X2 != 1 {
		t.Fatalf("links %+v", w.Links)
	}
	if o := w.Observer; o == nil || o.Revision != 0 || o.Z != 7.5 || o.TargetZ != 1 {
		t.Fatalf("home observer %+v", o)
	}

	commandWhenIdle(t, c, s.ID, "orbit-down 90 zoom 1")
	c.MustPost(query, &q, client.Var("id", s.ID))
	if o := q.Session.World.Observer; o == nil || o.Revision != 2 || o.Y >= 0 || o.Z != 1 {
		t.Fatalf("observer after orbit %+v", o)
	}

	// A 2D world keeps the 3D fields empty.
	s2 := create(t, c, "fire.nlogo")
	q.Session.World = world{}
	c.MustPost(query, &q, client.Var("id", s2.ID))
	w = q.Session.World
	if w.Depth != 1 || w.MinPzcor != 0 || w.MaxPzcor != 0 || w.WrapZ || w.Voxels != "" || w.Observer != nil {
		t.Fatalf("2D world 3D fields %+v", w)
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

func TestSetFrameRateClampAndDrops(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "wolfsheep.nlogo")
	query := `query ($id: ID!) {
		session(id: $id) { frameRateCap droppedFrames droppedSlow publishedFps tps ticks running }
		worldSettings(sessionId: $id) { frameRateCap }
	}`
	type sample struct {
		Session struct {
			FrameRateCap  int
			DroppedFrames int
			DroppedSlow   int
			PublishedFps  float64
			Tps           float64
			Ticks         *float64
			Running       bool
		}
		WorldSettings struct {
			FrameRateCap int
		}
	}
	var q sample
	c.MustPost(query, &q, client.Var("id", s.ID))
	if q.Session.FrameRateCap != 24 || q.WorldSettings.FrameRateCap != 24 || q.Session.DroppedFrames != 0 {
		t.Fatalf("default session %+v settings %d", q.Session, q.WorldSettings.FrameRateCap)
	}
	set := func(fps int) int {
		t.Helper()
		var resp struct {
			SetFrameRate struct{ FrameRateCap int }
		}
		c.MustPost(`mutation ($id: ID!, $fps: Int!) {
			setFrameRate(sessionId: $id, fps: $fps) { frameRateCap }
		}`, &resp, client.Var("id", s.ID), client.Var("fps", fps))
		return resp.SetFrameRate.FrameRateCap
	}
	if got := set(0); got != 1 {
		t.Fatalf("fps 0 clamped to %d", got)
	}
	if got := set(500); got != 120 {
		t.Fatalf("fps 500 clamped to %d", got)
	}
	if got := set(24); got != 24 {
		t.Fatalf("fps 24 became %d", got)
	}

	setup(t, c, s.ID)
	c.MustPost(`mutation ($id: ID!) { setSpeed(sessionId: $id, speed: 100) { id } }`, &struct {
		SetSpeed struct{ ID string }
	}{}, client.Var("id", s.ID))

	run := func(fps int) sample {
		t.Helper()
		if got := set(fps); got != fps {
			t.Fatalf("set %d -> %d", fps, got)
		}
		c.MustPost(`mutation ($id: ID!) { go(sessionId: $id) { id } }`, &struct {
			Go struct{ ID string }
		}{}, client.Var("id", s.ID))
		time.Sleep(2 * time.Second)
		var got sample
		c.MustPost(query, &got, client.Var("id", s.ID))
		c.MustPost(`mutation ($id: ID!) { stop(sessionId: $id) { id } }`, &struct {
			Stop struct{ ID string }
		}{}, client.Var("id", s.ID))
		if !got.Session.Running || got.Session.FrameRateCap != fps || got.Session.Ticks == nil || *got.Session.Ticks <= 0 {
			t.Fatalf("cap %d sample %+v", fps, got.Session)
		}
		t.Logf("cap %d tps %.2f ticks %.0f dropped %d slow %d publishedFps %.2f",
			fps, got.Session.Tps, *got.Session.Ticks, got.Session.DroppedFrames, got.Session.DroppedSlow, got.Session.PublishedFps)
		return got
	}
	at24 := run(24)
	// Stop returns before the halted step leaves the interpreter. Setup
	// waits that out. If it is still refused, retry until the tick ends.
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := c.Post(`mutation ($id: ID!) { setup(sessionId: $id) { id } }`, &struct {
			Setup struct{ ID string }
		}{}, client.Var("id", s.ID))
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "Stop the model before") || time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	at60 := run(60)
	if at24.Session.DroppedFrames == 0 && at60.Session.DroppedFrames == 0 {
		t.Log("model did not outrun either cap during the sample")
	} else if at24.Session.DroppedFrames <= at60.Session.DroppedFrames {
		t.Fatalf("expected more cap drops at 24 fps (%d) than 60 (%d)", at24.Session.DroppedFrames, at60.Session.DroppedFrames)
	}
}

func TestOutputAreaReachesSubscription(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "output.nlogo")
	sub := c.Websocket(`subscription ($id: ID!) { frames(sessionId: $id) {
		hasOutput outputAppend outputPartial outputReset outputEpoch outputSeq log
		widgets { kind x y width height fontSize }
	} }`, client.Var("id", s.ID))
	defer sub.Close()

	type frame struct {
		HasOutput     bool
		OutputAppend  []string
		OutputPartial string
		OutputReset   bool
		OutputEpoch   int
		OutputSeq     int
		Log           []string
		Widgets       []struct {
			Kind     string
			X        int
			Y        int
			Width    int
			Height   int
			FontSize int
		}
	}
	var msg struct{ Frames frame }
	if err := sub.Next(&msg); err != nil {
		t.Fatal(err)
	}
	var widgetOK bool
	for _, w := range msg.Frames.Widgets {
		if w.Kind != "output" {
			continue
		}
		widgetOK = true
		if w.X != 16 || w.Y != 110 || w.Width != 234 || w.Height != 120 || w.FontSize != 13 {
			t.Fatalf("output widget %+v", w)
		}
	}
	if !widgetOK || !msg.Frames.HasOutput {
		t.Fatalf("initial frame %+v", msg.Frames)
	}

	lines, epoch, seq := applyOutput(nil, -1, 0, msg.Frames.OutputReset, msg.Frames.OutputEpoch, msg.Frames.OutputSeq, msg.Frames.OutputAppend)
	setup(t, c, s.ID)
	deadline := time.Now().Add(2 * time.Second)
	for !containsLine(lines, "Output area ready.") {
		if time.Now().After(deadline) {
			t.Fatalf("subscription output %q", lines)
		}
		if err := sub.Next(&msg); err != nil {
			t.Fatal(err)
		}
		lines, epoch, seq = applyOutput(lines, epoch, seq, msg.Frames.OutputReset, msg.Frames.OutputEpoch, msg.Frames.OutputSeq, msg.Frames.OutputAppend)
		for _, line := range msg.Frames.Log {
			if strings.Contains(line, "Output area ready.") {
				t.Fatalf("output text landed in the command center: %q", msg.Frames.Log)
			}
		}
	}
	if !containsLine(lines, `next write is quoted: "widget"`) {
		t.Fatalf("output %q", lines)
	}

	var q struct {
		Session struct {
			HasOutput     bool
			Output        []string
			OutputPartial string
			Log           []string
		}
	}
	c.MustPost(`query ($id: ID!) { session(id: $id) { hasOutput output outputPartial log } }`, &q, client.Var("id", s.ID))
	if !q.Session.HasOutput || !containsLine(q.Session.Output, "Output area ready.") {
		t.Fatalf("session output %+v", q.Session)
	}

	var cmd struct {
		RunCommand struct {
			Ok    bool
			Error *string
		}
	}
	c.MustPost(`mutation ($id: ID!) { runCommand(sessionId: $id, text: "clear-all") { ok error } }`, &cmd, client.Var("id", s.ID))
	if !cmd.RunCommand.Ok {
		t.Fatalf("clear-all: %v", cmd.RunCommand.Error)
	}
	c.MustPost(`query ($id: ID!) { session(id: $id) { hasOutput output outputPartial log } }`, &q, client.Var("id", s.ID))
	if len(q.Session.Output) != 0 || q.Session.OutputPartial != "" {
		t.Fatalf("clear-all left %+v", q.Session)
	}
}

func TestOutputPrintFallsBackToLog(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")
	var cmd struct {
		RunCommand struct {
			Ok    bool
			Error *string
		}
	}
	c.MustPost(`mutation ($id: ID!) { runCommand(sessionId: $id, text: "output-print \"hi\"") { ok error } }`, &cmd, client.Var("id", s.ID))
	if !cmd.RunCommand.Ok {
		t.Fatalf("output-print: %v", cmd.RunCommand.Error)
	}
	var q struct {
		Session struct {
			HasOutput bool
			Output    []string
			Log       []string
		}
	}
	c.MustPost(`query ($id: ID!) { session(id: $id) { hasOutput output log } }`, &q, client.Var("id", s.ID))
	if q.Session.HasOutput || len(q.Session.Output) != 0 || !containsLine(q.Session.Log, "hi") {
		t.Fatalf("fallback %+v", q.Session)
	}
	c.MustPost(`mutation ($id: ID!) { runCommand(sessionId: $id, text: "clear-output") { ok error } }`, &cmd, client.Var("id", s.ID))
	if !cmd.RunCommand.Ok {
		t.Fatalf("clear-output: %v", cmd.RunCommand.Error)
	}
	c.MustPost(`query ($id: ID!) { session(id: $id) { hasOutput output log } }`, &q, client.Var("id", s.ID))
	if containsLine(q.Session.Log, "hi") {
		t.Fatalf("clear-output left the command center: %+v", q.Session.Log)
	}
}

func applyOutput(lines []string, epoch, seq int, reset bool, nextEpoch, nextSeq int, appendLines []string) ([]string, int, int) {
	if reset || nextEpoch != epoch || nextSeq < seq {
		lines = append([]string(nil), appendLines...)
	} else if nextSeq != seq {
		delta := nextSeq - seq
		extra := len(appendLines) - delta
		if extra > 0 {
			appendLines = appendLines[extra:]
		}
		lines = append(lines, appendLines...)
	}
	if len(lines) > 2000 {
		lines = append([]string(nil), lines[len(lines)-2000:]...)
	}
	return lines, nextEpoch, nextSeq
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

// commandWhenIdle retries a command-center line while a halted step is
// still inside the interpreter. A command during that window is refused.
func commandWhenIdle(t *testing.T, c *client.Client, id, text string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last string
	for {
		var cmd struct {
			RunCommand struct {
				Ok    bool
				Error *string
			}
		}
		c.MustPost(`mutation ($id: ID!, $text: String!) {
			runCommand(sessionId: $id, text: $text) { ok error }
		}`, &cmd, client.Var("id", id), client.Var("text", text))
		if cmd.RunCommand.Ok {
			return
		}
		if cmd.RunCommand.Error != nil {
			last = *cmd.RunCommand.Error
		}
		if !strings.Contains(last, "Stop the model before") || time.Now().After(deadline) {
			t.Fatalf("%s: %s", text, last)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSessionInfo(t *testing.T) {
	c := newClient(t)
	var created struct {
		CreateSession struct {
			ID       string
			Info     string
			ModelDir *string
		}
	}
	c.MustPost(`mutation ($m: String!) {
		createSession(model: $m) { id info modelDir }
	}`, &created, client.Var("m", "fire.nlogo"))
	if !strings.Contains(created.CreateSession.Info, "orthogonal neighbor") {
		t.Fatalf("fire info %q", created.CreateSession.Info)
	}
	if created.CreateSession.ModelDir == nil || *created.CreateSession.ModelDir != "" {
		t.Fatalf("embedded model dir %#v, want the models root", created.CreateSession.ModelDir)
	}

	nlogox := `<?xml version="1.0"?>
<model version="NetLogo 7.0.0">
  <code><![CDATA[to setup
  clear-all
end
]]></code>
  <info><![CDATA[## WHAT IS IT?

A .nlogox info paragraph.
]]></info>
</model>`
	var loaded struct {
		LoadModelSource struct {
			Info     string
			ModelDir *string
		}
	}
	c.MustPost(`mutation ($id: ID!, $name: String!, $src: String!) {
		loadModelSource(sessionId: $id, filename: $name, source: $src) { info modelDir }
	}`, &loaded, client.Var("id", created.CreateSession.ID), client.Var("name", "pack.nlogox"), client.Var("src", nlogox))
	if !strings.Contains(loaded.LoadModelSource.Info, "A .nlogox info paragraph.") {
		t.Fatalf("nlogox info %q", loaded.LoadModelSource.Info)
	}
	if loaded.LoadModelSource.ModelDir != nil {
		t.Fatalf("uploaded model dir %#v, want null", *loaded.LoadModelSource.ModelDir)
	}

	dir := t.TempDir()
	t.Setenv("MODELS_DIR", dir)
	if err := os.MkdirAll(filepath.Join(dir, "biology"), 0o755); err != nil {
		t.Fatal(err)
	}
	nlogo := "to setup\n  clear-all\nend\n@#$#@#$#@\n@#$#@#$#@\nFrom the .nlogo file.\n@#$#@#$#@\n"
	if err := os.WriteFile(filepath.Join(dir, "biology", "pack.nlogo"), []byte(nlogo), 0o644); err != nil {
		t.Fatal(err)
	}
	var fromFile struct {
		LoadModelFile struct {
			Info     string
			ModelDir *string
		}
	}
	c.MustPost(`mutation ($id: ID!, $path: String!) {
		loadModelFile(sessionId: $id, path: $path) { info modelDir }
	}`, &fromFile, client.Var("id", created.CreateSession.ID), client.Var("path", "biology/pack.nlogo"))
	if !strings.Contains(fromFile.LoadModelFile.Info, "From the .nlogo file.") {
		t.Fatalf("file info %q", fromFile.LoadModelFile.Info)
	}
	if fromFile.LoadModelFile.ModelDir == nil || *fromFile.LoadModelFile.ModelDir != "biology" {
		t.Fatalf("model dir %#v", fromFile.LoadModelFile.ModelDir)
	}
}

func setup(t *testing.T, c *client.Client, id string) created {
	t.Helper()
	var resp struct {
		Setup created
	}
	c.MustPost(`mutation ($id: ID!) { setup(sessionId: $id) { id ticksStarted ticks widgets { index kind variable } } }`, &resp, client.Var("id", id))
	return resp.Setup
}
