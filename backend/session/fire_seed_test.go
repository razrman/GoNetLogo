package session

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"testing"
	"time"

	"gonetlogo/pkg/nl"
)

const fireModelPath = "library/Sample Models/Earth Science/Fire.nlogox"
const fireTicks = 40

// TestFireSeedOverride loads the library Fire model and checks that a
// session seed override repeats the burned count and patch colors.
func TestFireSeedOverride(t *testing.T) {
	t.Run("setup twice", func(t *testing.T) {
		s := loadFire(t)
		setOverrideSeed(t, s, "42")
		first := runFire(t, s)
		second := runFire(t, s)
		if first != second {
			t.Fatalf("override on setup twice = %+v then %+v", first, second)
		}
		if first.burned == 0 || first.patches == 0 {
			t.Fatalf("empty run %+v", first)
		}
	})

	t.Run("two sessions", func(t *testing.T) {
		one := fireSession(t, "42")
		two := fireSession(t, "42")
		if one != two {
			t.Fatalf("two sessions = %+v and %+v", one, two)
		}
	})

	t.Run("model reload", func(t *testing.T) {
		s := loadFire(t)
		setOverrideSeed(t, s, "42")
		before := runFire(t, s)
		if err := s.LoadModelFile(fireModelPath); err != nil {
			t.Fatal(err)
		}
		after := runFire(t, s)
		if before != after {
			t.Fatalf("reload = %+v then %+v", before, after)
		}
	})

	t.Run("override off diverges", func(t *testing.T) {
		s := loadFire(t)
		first := runFire(t, s)
		second := runFire(t, s)
		if first == second {
			t.Fatal("override off setup twice produced the same fire")
		}
	})
}

type fireFinger struct {
	burned  int
	patches int
	hash    uint64
}

func fireSession(t *testing.T, seed string) fireFinger {
	t.Helper()
	s := loadFire(t)
	setOverrideSeed(t, s, seed)
	return runFire(t, s)
}

func loadFire(t *testing.T) *Session {
	t.Helper()
	s := newSession()
	t.Cleanup(s.Close)
	if err := s.LoadModelFile(fireModelPath); err != nil {
		t.Fatal(err)
	}
	return s
}

func runFire(t *testing.T, s *Session) fireFinger {
	t.Helper()
	start := time.Now()
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < fireTicks; i++ {
		if err := s.Step(); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}
	got := fireFingerprint(t, s)
	t.Logf("fire %d ticks in %s burned=%d patches=%d", fireTicks, time.Since(start).Round(time.Millisecond), got.burned, got.patches)
	return got
}

func fireFingerprint(t *testing.T, s *Session) fireFinger {
	t.Helper()
	s.mu.Lock()
	vm := s.vm
	s.mu.Unlock()
	if vm == nil || vm.World == nil {
		t.Fatal("no world")
	}
	var out fireFinger
	vm.DoLocked(func() {
		if v, ok := vm.World.Global("burned-trees"); ok {
			if n, err := v.Num(); err == nil {
				out.burned = int(n)
			}
		}
		h := fnv.New64a()
		var buf [8]byte
		for _, a := range vm.World.Patches().Agents {
			p, ok := a.(*nl.Patch)
			if !ok {
				continue
			}
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(p.PColor))
			_, _ = h.Write(buf[:])
			out.patches++
		}
		out.hash = h.Sum64()
	})
	return out
}
