package graph

import (
	"strconv"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"

	"gonetlogo/pkg/nl"
)

func TestRNGSeedApplyAndGenerate(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")

	type bounds struct {
		MinPxcor  int
		MaxPxcor  int
		MinPycor  int
		MaxPycor  int
		PatchSize float64
		WrapX     bool
		WrapY     bool
		RngSeed   string
	}
	var cur struct{ WorldSettings bounds }
	c.MustPost(`query ($id: ID!) {
		worldSettings(sessionId: $id) {
			minPxcor maxPxcor minPycor maxPycor patchSize wrapX wrapY rngSeed
		}
	}`, &cur, client.Var("id", s.ID))
	start := cur.WorldSettings
	if _, err := nl.ParseRNGSeed(start.RngSeed); err != nil {
		t.Fatalf("initial rngSeed %q: %v", start.RngSeed, err)
	}

	setSeed := func(minPx int, seed *string) (string, int, error) {
		t.Helper()
		var resp struct {
			SetWorld struct {
				World struct {
					RngSeed  string
					MinPxcor int
				}
			}
		}
		err := c.Post(`mutation ($id: ID!, $minPx: Int!, $maxPx: Int!, $minPy: Int!, $maxPy: Int!, $ps: Float!, $wrapX: Boolean!, $wrapY: Boolean!, $seed: String) {
			setWorld(sessionId: $id, minPxcor: $minPx, maxPxcor: $maxPx, minPycor: $minPy, maxPycor: $maxPy, patchSize: $ps, wrapX: $wrapX, wrapY: $wrapY, rngSeed: $seed) {
				world { rngSeed minPxcor }
			}
		}`, &resp,
			client.Var("id", s.ID),
			client.Var("minPx", minPx),
			client.Var("maxPx", start.MaxPxcor),
			client.Var("minPy", start.MinPycor),
			client.Var("maxPy", start.MaxPycor),
			client.Var("ps", start.PatchSize),
			client.Var("wrapX", start.WrapX),
			client.Var("wrapY", start.WrapY),
			client.Var("seed", seed),
		)
		return resp.SetWorld.World.RngSeed, resp.SetWorld.World.MinPxcor, err
	}

	nextMin := start.MinPxcor + 1
	if nextMin > 0 {
		nextMin = start.MinPxcor - 1
	}
	bad := "NOPE!!"
	if _, _, err := setSeed(nextMin, &bad); err == nil || !strings.Contains(err.Error(), "4294967295") {
		t.Fatalf("invalid seed error = %v", err)
	}
	negative := "-1"
	if _, _, err := setSeed(nextMin, &negative); err == nil || !strings.Contains(err.Error(), "4294967295") {
		t.Fatalf("negative seed error = %v", err)
	}
	tooBig := "4294967296"
	if _, _, err := setSeed(nextMin, &tooBig); err == nil || !strings.Contains(err.Error(), "4294967295") {
		t.Fatalf("overflow seed error = %v", err)
	}
	c.MustPost(`query ($id: ID!) { worldSettings(sessionId: $id) { minPxcor rngSeed } }`, &cur, client.Var("id", s.ID))
	if cur.WorldSettings.MinPxcor != start.MinPxcor || cur.WorldSettings.RngSeed != start.RngSeed {
		t.Fatalf("invalid seed changed the world: %+v", cur.WorldSettings)
	}

	known := "1" // random 100 then yields 81
	gotSeed, gotMin, err := setSeed(nextMin, &known)
	if err != nil {
		t.Fatal(err)
	}
	if gotSeed != known || gotMin != nextMin {
		t.Fatalf("applied seed %q min %d", gotSeed, gotMin)
	}
	if got := random100(t, c, s.ID); got != "81" {
		t.Fatalf("random 100 after %s = %q, want 81", known, got)
	}
	if got := random100(t, c, s.ID); got != "88" {
		t.Fatalf("second random 100 = %q, want 88", got)
	}

	// A resize that fails must not reseed. The next draw stays on this
	// sequence (80), rather than restarting at 81.
	outside := 1
	outsideSeed := known
	if _, _, err := setSeed(outside, &outsideSeed); err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("origin error = %v", err)
	}
	if got := random100(t, c, s.ID); got != "80" {
		t.Fatalf("random 100 after rejected resize = %q, want 80", got)
	}

	seen := map[string]bool{}
	var generated string
	for i := 0; i < 5; i++ {
		var resp struct{ GenerateRngSeed string }
		c.MustPost(`mutation ($id: ID!) { generateRngSeed(sessionId: $id) }`, &resp, client.Var("id", s.ID))
		if _, err := nl.ParseRNGSeed(resp.GenerateRngSeed); err != nil {
			t.Fatalf("generated %q: %v", resp.GenerateRngSeed, err)
		}
		seen[resp.GenerateRngSeed] = true
		generated = resp.GenerateRngSeed
	}
	if len(seen) < 2 {
		t.Fatalf("generateRngSeed returned one value: %v", seen)
	}

	var after struct {
		WorldSettings bounds
		Session       struct{ World bounds }
	}
	c.MustPost(`query ($id: ID!) {
		worldSettings(sessionId: $id) { minPxcor maxPxcor rngSeed }
		session(id: $id) { world { rngSeed minPxcor } }
	}`, &after, client.Var("id", s.ID))
	if after.WorldSettings.RngSeed != generated || after.Session.World.RngSeed != generated {
		t.Fatalf("seed not stored: settings %q world %q generated %q", after.WorldSettings.RngSeed, after.Session.World.RngSeed, generated)
	}
	if after.WorldSettings.MinPxcor != nextMin || after.Session.World.MinPxcor != nextMin {
		t.Fatalf("generate resized the world: %+v", after.WorldSettings)
	}
	n, err := nl.ParseRNGSeed(generated)
	if err != nil {
		t.Fatal(err)
	}
	fresh := nl.NewWorld(0, 0, 0, 0, false, false)
	fresh.Seed(n)
	want, err := fresh.RandomInt(100)
	if err != nil {
		t.Fatal(err)
	}
	if got := random100(t, c, s.ID); got != strconv.Itoa(want) {
		t.Fatalf("random 100 after generate %s = %q, want %d", generated, got, want)
	}
}

func TestSeedOverrideRoundTrip(t *testing.T) {
	c := newClient(t)
	s := create(t, c, "fire.nlogo")
	type view struct {
		MinPxcor, MaxPxcor, MinPycor, MaxPycor int
		PatchSize                              float64
		WrapX, WrapY, SeedOverride             bool
		RngSeed                                string
	}
	var cur struct{ WorldSettings view }
	c.MustPost(`query ($id: ID!) {
		worldSettings(sessionId: $id) {
			minPxcor maxPxcor minPycor maxPycor patchSize wrapX wrapY rngSeed seedOverride
		}
	}`, &cur, client.Var("id", s.ID))
	if cur.WorldSettings.SeedOverride {
		t.Fatal("seed override starts on")
	}

	var turned struct {
		SetSeedOverride struct {
			World struct {
				SeedOverride bool
				RngSeed      string
			}
		}
	}
	c.MustPost(`mutation ($id: ID!, $on: Boolean!) {
		setSeedOverride(sessionId: $id, on: $on) { world { seedOverride rngSeed } }
	}`, &turned, client.Var("id", s.ID), client.Var("on", true))
	if !turned.SetSeedOverride.World.SeedOverride || turned.SetSeedOverride.World.RngSeed == "" {
		t.Fatalf("override on: %+v", turned.SetSeedOverride.World)
	}

	start := cur.WorldSettings
	apply := func(seed string) string {
		t.Helper()
		var resp struct {
			SetWorld struct {
				World struct {
					RngSeed      string
					SeedOverride bool
					MinPxcor     int
				}
			}
		}
		c.MustPost(`mutation ($id: ID!, $minPx: Int!, $maxPx: Int!, $minPy: Int!, $maxPy: Int!, $ps: Float!, $wrapX: Boolean!, $wrapY: Boolean!, $seed: String, $on: Boolean) {
			setWorld(sessionId: $id, minPxcor: $minPx, maxPxcor: $maxPx, minPycor: $minPy, maxPycor: $maxPy, patchSize: $ps, wrapX: $wrapX, wrapY: $wrapY, rngSeed: $seed, seedOverride: $on) {
				world { rngSeed seedOverride minPxcor }
			}
		}`, &resp,
			client.Var("id", s.ID),
			client.Var("minPx", start.MinPxcor),
			client.Var("maxPx", start.MaxPxcor),
			client.Var("minPy", start.MinPycor),
			client.Var("maxPy", start.MaxPycor),
			client.Var("ps", start.PatchSize),
			client.Var("wrapX", start.WrapX),
			client.Var("wrapY", start.WrapY),
			client.Var("seed", seed),
			client.Var("on", true),
		)
		if !resp.SetWorld.World.SeedOverride || resp.SetWorld.World.RngSeed != seed || resp.SetWorld.World.MinPxcor != start.MinPxcor {
			t.Fatalf("applied %+v", resp.SetWorld.World)
		}
		return resp.SetWorld.World.RngSeed
	}
	apply("1")
	if got := random100(t, c, s.ID); got != "81" {
		t.Fatalf("random 100 = %q, want 81", got)
	}
	if got := random100(t, c, s.ID); got != "88" {
		t.Fatalf("second random 100 = %q, want 88", got)
	}
	// The same seed text must reseed again. Omitting it would have left 80.
	apply("1")
	if got := random100(t, c, s.ID); got != "81" {
		t.Fatalf("random 100 after repeat apply = %q, want 81", got)
	}
}

func random100(t *testing.T, c *client.Client, id string) string {
	t.Helper()
	var resp struct {
		RunCommand struct {
			Ok     bool
			Output *string
			Error  *string
		}
	}
	c.MustPost(`mutation ($id: ID!) { runCommand(sessionId: $id, text: "random 100") { ok output error } }`, &resp, client.Var("id", id))
	if !resp.RunCommand.Ok || resp.RunCommand.Output == nil {
		t.Fatalf("random 100: ok=%v output=%v err=%v", resp.RunCommand.Ok, resp.RunCommand.Output, resp.RunCommand.Error)
	}
	return strings.TrimSpace(*resp.RunCommand.Output)
}
