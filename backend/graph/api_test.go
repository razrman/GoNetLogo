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

	var cmd struct {
		RunCommand struct {
			Ok    bool
			Error *string
		}
	}
	c.MustPost(`mutation ($id: ID!) { runCommand(sessionId: $id, text: "reset-ticks") { ok error } }`, &cmd, client.Var("id", s.ID))
	if !cmd.RunCommand.Ok {
		t.Fatalf("reset-ticks: %v", cmd.RunCommand.Error)
	}
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
	c.MustPost(`mutation ($id: ID!) { runCommand(sessionId: $id, text: "clear-all") { ok error } }`, &cmd, client.Var("id", s.ID))
	if !cmd.RunCommand.Ok {
		t.Fatalf("clear-all: %v", cmd.RunCommand.Error)
	}
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
	setup(t, c, s.ID)
	at60 := run(60)
	if at24.Session.DroppedFrames == 0 && at60.Session.DroppedFrames == 0 {
		t.Log("model did not outrun either cap during the sample")
	} else if at24.Session.DroppedFrames <= at60.Session.DroppedFrames {
		t.Fatalf("expected more cap drops at 24 fps (%d) than 60 (%d)", at24.Session.DroppedFrames, at60.Session.DroppedFrames)
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
