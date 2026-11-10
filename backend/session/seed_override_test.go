package session

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"strings"
	"testing"

	"gonetlogo/pkg/nl"
)

const seedModelTail = `
@#$#@#$#@
GRAPHICS-WINDOW
280
10
400
130
-1
-1
12.0
1
10
1
1
1
0
0
0
1
-2
2
-2
2
1
1
1
ticks
30.0

BUTTON
16
20
80
53
setup
setup
NIL
1
T
OBSERVER
NIL
NIL
NIL
NIL
1

BUTTON
90
20
154
53
go
go
T
1
T
OBSERVER
NIL
NIL
NIL
NIL
0
@#$#@#$#@
@#$#@#$#@
`

const randomPatchModel = `to setup
  clear-all
  ask patches [ set pcolor random 10 ]
  reset-ticks
end

to go
  tick
end
` + seedModelTail

const randomSeedModel = `to setup
  clear-all
  random-seed 1
  reset-ticks
end

to go
  tick
end
` + seedModelTail

func TestSeedOverrideRepeatsSetup(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.useModel("seed.nlogo", randomPatchModel); err != nil {
		t.Fatal(err)
	}
	setOverrideSeed(t, s, "42")
	first := setupFingerprint(t, s)
	second := setupFingerprint(t, s)
	if first.hash != second.hash || first.n != second.n {
		t.Fatalf("override on setup twice = %+v then %+v", first, second)
	}
	if first.n == 0 {
		t.Fatal("no patches")
	}
}

func TestSeedOverrideOffDiverges(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.useModel("seed.nlogo", randomPatchModel); err != nil {
		t.Fatal(err)
	}
	first := setupFingerprint(t, s)
	second := setupFingerprint(t, s)
	if first.hash == second.hash {
		t.Fatal("override off setup twice produced the same forest")
	}
}

func TestSeedOverrideAcrossSessionsAndReload(t *testing.T) {
	one := runOverrideSession(t, "42")
	two := runOverrideSession(t, "42")
	if one.hash != two.hash || one.n != two.n {
		t.Fatalf("two sessions = %+v and %+v", one, two)
	}

	s := newSession()
	defer s.Close()
	if err := s.LoadModelSource("seed.nlogo", randomPatchModel); err != nil {
		t.Fatal(err)
	}
	setOverrideSeed(t, s, "42")
	before := setupFingerprint(t, s)
	if err := s.LoadModelSource("seed.nlogo", randomPatchModel); err != nil {
		t.Fatal(err)
	}
	after := setupFingerprint(t, s)
	if before.hash != after.hash || before.n != after.n {
		t.Fatalf("reload = %+v then %+v", before, after)
	}
	if before.hash != one.hash {
		t.Fatalf("reload fingerprint %+v, fresh session %+v", before, one)
	}
}

func TestModelRandomSeedWinsOverOverride(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.useModel("seed.nlogo", randomSeedModel); err != nil {
		t.Fatal(err)
	}
	setOverrideSeed(t, s, "99")
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	// random-seed 1 inside setup runs after the override, so random 100 is 81.
	if got := commandRandom(t, s); got != "81" {
		t.Fatalf("random 100 after setup = %q, want 81", got)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	if got := commandRandom(t, s); got != "81" {
		t.Fatalf("second setup random 100 = %q, want 81", got)
	}
}

func TestOverrideDoesNotReseedUntilSetupOrApply(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.useModel("seed.nlogo", randomPatchModel); err != nil {
		t.Fatal(err)
	}
	setOverrideSeed(t, s, "1")
	if got := commandRandom(t, s); got != "81" {
		t.Fatalf("first random 100 = %q", got)
	}
	if got := commandRandom(t, s); got != "88" {
		t.Fatalf("second random 100 = %q", got)
	}
	// Turning the override off and on records the seed and does not reseed.
	if err := s.SetSeedOverride(false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSeedOverride(true); err != nil {
		t.Fatal(err)
	}
	if got := commandRandom(t, s); got != "80" {
		t.Fatalf("random 100 after toggling override = %q, want 80", got)
	}
	// Applying the same seed again reseeds now, even though the text is unchanged.
	setOverrideSeed(t, s, "1")
	if got := commandRandom(t, s); got != "81" {
		t.Fatalf("random 100 after repeat apply = %q, want 81", got)
	}
}

func TestSeedOnlyChangeWhileRunning(t *testing.T) {
	s := newSession()
	defer s.Close()
	if err := s.useModel("seed.nlogo", randomPatchModel); err != nil {
		t.Fatal(err)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	if err := s.Go(); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSeedOverride(true); err != nil {
		t.Fatalf("override during go: %v", err)
	}
	settings, err := s.WorldSettings()
	if err != nil {
		t.Fatal(err)
	}
	seed := "7"
	on := true
	if err := s.SetWorld(settings.MinPxcor, settings.MaxPxcor, settings.MinPycor, settings.MaxPycor, settings.PatchSize, settings.WrapX, settings.WrapY, nil, nil, nil, &seed, &on); err != nil {
		t.Fatalf("seed during go: %v", err)
	}
	if !s.Session().Running {
		t.Fatal("seeding stopped the run")
	}
	got, err := s.WorldSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !got.SeedOverride || got.RngSeed != "7" || got.MaxPxcor != settings.MaxPxcor {
		t.Fatalf("settings during go = %+v", got)
	}
	if err := s.SetWorld(settings.MinPxcor, settings.MaxPxcor+1, settings.MinPycor, settings.MaxPycor, settings.PatchSize, settings.WrapX, settings.WrapY, nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "Stop the model before changing the world.") {
		t.Fatalf("resize during go: %v", err)
	}
}

type patchFinger struct {
	hash uint64
	n    int
}

func runOverrideSession(t *testing.T, seed string) patchFinger {
	t.Helper()
	s := newSession()
	defer s.Close()
	if err := s.useModel("seed.nlogo", randomPatchModel); err != nil {
		t.Fatal(err)
	}
	setOverrideSeed(t, s, seed)
	return setupFingerprint(t, s)
}

func setOverrideSeed(t *testing.T, s *Session, seed string) {
	t.Helper()
	if err := s.SetSeedOverride(true); err != nil {
		t.Fatal(err)
	}
	settings, err := s.WorldSettings()
	if err != nil {
		t.Fatal(err)
	}
	on := true
	text := seed
	if err := s.SetWorld(settings.MinPxcor, settings.MaxPxcor, settings.MinPycor, settings.MaxPycor, settings.PatchSize, settings.WrapX, settings.WrapY, nil, nil, nil, &text, &on); err != nil {
		t.Fatal(err)
	}
}

func setupFingerprint(t *testing.T, s *Session) patchFinger {
	t.Helper()
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	vm := s.vm
	s.mu.Unlock()
	var out patchFinger
	vm.DoLocked(func() {
		h := fnv.New64a()
		var buf [8]byte
		for _, a := range vm.World.Patches().Agents {
			p, ok := a.(*nl.Patch)
			if !ok {
				continue
			}
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(p.PColor))
			_, _ = h.Write(buf[:])
			out.n++
		}
		out.hash = h.Sum64()
	})
	return out
}

func commandRandom(t *testing.T, s *Session) string {
	t.Helper()
	out, err := s.RunCommand("random 100")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}
