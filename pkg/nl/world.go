package nl

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// World is the NetLogo world: a patch grid, turtles, links, and the observer.
type World struct {
	MinPxcor, MaxPxcor int
	MinPycor, MaxPycor int
	// MinPzcor and MaxPzcor are the Z bounds. A 2D world keeps both at 0,
	// so it has depth 1 and every 2D path sees one flat layer.
	MinPzcor, MaxPzcor int
	WrapX, WrapY       bool
	// WrapZ is meaningful only in a 3D world. NetLogo 3D worlds are tori,
	// so NewWorld3D turns on wrapping on every axis.
	WrapZ     bool
	PatchSize float64
	// LabelFont is the view's label point size at PatchSize. NetLogo's
	// default is 10. Zero is treated as that default when painting.
	LabelFont int
	// Shapes is the turtle shape library. Nil falls back to the built-in
	// approximations. Models replace DefaultShapes with their own section.
	Shapes *ShapeList

	width, height int
	// depth is the number of patch layers. It is 1 in a 2D world.
	depth int
	// threeD marks a NetLogo 3D world. 3D-only primitives check it, and
	// the 2D code paths are taken whenever it is false.
	threeD bool
	// The 3D observer camera: position, target, spherical pose around the
	// target, and a revision bumped by every observer camera command. See
	// world3d.go.
	obsX, obsY, obsZ        float64
	obsTX, obsTY, obsTZ     float64
	obsDist, obsAz, obsElev float64
	obsRev                  uint64
	patches                 []*Patch
	// patchOrder is NetLogo's patch iteration order: top to bottom, left to right.
	patchOrder []*Patch
	// patchSet is the cached canonical patches agentset. Patches are not
	// created or destroyed except by resize, so one-of patches can index it.
	patchSet *AgentSet

	turtles map[int]*Turtle
	whoSeq  []int // ascending who, including dead slots compacted lazily
	nextWho int
	// breedLive is the living turtles of each breed, in insertion order.
	// ask uses it instead of scanning every turtle. Nil after clear-turtles.
	breedLive map[string][]*Turtle

	links   []*Link
	nextLID int

	globals    map[string]Value
	ifaceGlob  map[string]bool
	turtlesOwn []string
	patchesOwn []string
	linksOwn   []string
	breedOwn   map[string][]string // plural -> vars
	breeds     map[string]*breedDef
	linkBreeds map[string]*linkBreedDef
	breedCalls map[string]breedCallDef

	defaultShape map[string]string // breed plural or "turtles" -> shape

	ticks        float64
	ticksStarted bool
	// runEpoch increments on clear-ticks, reset-ticks, and clear-all.
	// The run timer watches it. Tick does not, so a benchmark is unchanged.
	runEpoch uint64

	perspectiveAgent Agent
	perspectiveMode  string // "watch", "follow", "ride", or ""

	rng      *MersenneTwister
	rngDraws atomic.Uint64
	seed     int64

	// drawing is the pen and stamp layer, oldest first, capped so a long
	// pen-down run cannot retain every stroke. Turtle, Patch, and Link
	// labels keep their value type. The view formats them with
	// DisplayLabel when it paints. label-color and plabel-color stay in Vars.
	drawing drawingLog

	// mu guards mutation of the world against a concurrent render.
	// The desktop draw thread never holds it: it paints the last view
	// published by unlockPublish. Interpreter entry points hold it for
	// one command, then publish.
	mu   sync.Mutex
	view atomic.Pointer[ViewSnap]
	// dirty means the published view is behind the world. Commands set it
	// instead of copying the grid, and the painter copies once per frame.
	dirty atomic.Bool
}

type colorRGBA struct{ R, G, B, A uint8 }

type breedCallKind int

const (
	breedCallLinkNeighbor breedCallKind = iota
	breedCallIsBreed
	breedCallOn
	breedCallAt
	breedCallSingular
	breedCallLinkSingular
	breedCallLinkWith
	breedCallInLinkFrom
	breedCallOutLinkTo
	breedCallMyLinks
)

type breedCallDef struct {
	kind     breedCallKind
	breed    string
	dir      int
	directed bool
}

type breedDef struct {
	Plural   string
	Singular string
	Own      []string
}

type linkBreedDef struct {
	Plural   string
	Singular string
	Directed bool
	Own      []string
}

// Patch is one cell of the grid.
type Patch struct {
	PX, PY  int
	PZ      int // always 0 in a 2D world
	PColor  float64
	RGB     color.RGBA
	HasRGB  bool
	Vars    map[string]Value
	Label   Value
	turtles []*Turtle
	world   *World
	dead    bool
}

func (p *Patch) AgentKind() AgentKind { return KindPatch }
func (p *Patch) IsDead() bool         { return p == nil || p.dead }
func (p *Patch) AgentString() string {
	if p.world != nil && p.world.threeD {
		return fmt.Sprintf("(patch %d %d %d)", p.PX, p.PY, p.PZ)
	}
	return fmt.Sprintf("(patch %d %d)", p.PX, p.PY)
}

// Turtle is a mobile agent.
type Turtle struct {
	Who     int
	X, Y    float64
	Heading float64
	// Z, Pitch, and Roll are the 3D position and orientation. They stay 0
	// in a 2D world. Pitch and Roll are degrees in [0, 360).
	Z, Pitch, Roll float64
	Color          float64
	RGB            color.RGBA
	HasRGB         bool
	Shape          string
	Size           float64
	Hidden         bool
	Label          Value
	Breed          string // plural; empty means the default turtles breed
	PenDown        bool
	// PenErase is pen-mode "erase": movement removes drawing instead of
	// adding it. PenDown and PenErase are not both true.
	PenErase bool
	PenSize  float64
	Vars     map[string]Value
	links    []*Link
	dead     bool
	world    *World
	patch    *Patch
}

func (t *Turtle) AgentKind() AgentKind { return KindTurtle }
func (t *Turtle) IsDead() bool         { return t == nil || t.dead }
func (t *Turtle) AgentString() string {
	if t.Breed != "" {
		// singular is not stored; "turtle N" is always acceptable,
		// breed name is available via the breed reporter.
	}
	return fmt.Sprintf("(turtle %d)", t.Who)
}

// Link connects two turtles.
type Link struct {
	ID        int
	End1      *Turtle
	End2      *Turtle
	Directed  bool
	Breed     string
	Color     float64
	RGB       color.RGBA
	HasRGB    bool
	Shape     string
	Thickness float64
	Hidden    bool
	Label     Value
	TieMode   string
	Vars      map[string]Value
	dead      bool
	world     *World
}

func (l *Link) AgentKind() AgentKind { return KindLink }
func (l *Link) IsDead() bool         { return l == nil || l.dead }
func (l *Link) AgentString() string {
	a, b := -1, -1
	if l.End1 != nil {
		a = l.End1.Who
	}
	if l.End2 != nil {
		b = l.End2.Who
	}
	return fmt.Sprintf("(link %d %d)", a, b)
}

// NewWorld builds a world. The default NetLogo size is -16..16 on both axes, wrapping.
func NewWorld(minPx, maxPx, minPy, maxPy int, wrapX, wrapY bool) *World {
	w := &World{
		MinPxcor:     minPx,
		MaxPxcor:     maxPx,
		MinPycor:     minPy,
		MaxPycor:     maxPy,
		WrapX:        wrapX,
		WrapY:        wrapY,
		PatchSize:    12,
		LabelFont:    10,
		Shapes:       DefaultShapes(),
		turtles:      map[int]*Turtle{},
		globals:      map[string]Value{},
		ifaceGlob:    map[string]bool{},
		breeds:       map[string]*breedDef{},
		linkBreeds:   map[string]*linkBreedDef{},
		breedCalls:   map[string]breedCallDef{},
		breedOwn:     map[string][]string{},
		defaultShape: map[string]string{"turtles": "default"},
	}
	w.recreatePatches()
	w.Seed(time.Now().UnixNano())
	return w
}

func (w *World) recreatePatches() {
	w.patchSet = nil
	if !w.threeD {
		w.MinPzcor, w.MaxPzcor = 0, 0
	}
	w.width = w.MaxPxcor - w.MinPxcor + 1
	w.height = w.MaxPycor - w.MinPycor + 1
	w.depth = w.MaxPzcor - w.MinPzcor + 1
	if w.width < 1 || w.height < 1 || w.depth < 1 {
		panic("world size must be positive")
	}
	n := w.width * w.height * w.depth
	w.patches = make([]*Patch, n)
	w.patchOrder = make([]*Patch, 0, n)
	// NetLogo order is top to bottom, left to right. A 3D world lists the
	// highest layer first, the same way each layer lists its top row first.
	for z := w.MaxPzcor; z >= w.MinPzcor; z-- {
		for y := w.MaxPycor; y >= w.MinPycor; y-- {
			for x := w.MinPxcor; x <= w.MaxPxcor; x++ {
				p := &Patch{
					PX: x, PY: y, PZ: z,
					PColor: 0,
					Vars:   map[string]Value{},
					Label:  Str(""),
					world:  w,
				}
				for _, name := range w.patchesOwn {
					p.Vars[name] = Num(0)
				}
				w.patches[w.patchIndex3(x, y, z)] = p
				w.patchOrder = append(w.patchOrder, p)
			}
		}
	}
	w.drawing = drawingLog{}
}

// patchIndex is the slot of (px, py) on layer pzcor 0, the only layer of
// a 2D world.
func (w *World) patchIndex(px, py int) int {
	return w.patchIndex3(px, py, 0)
}

func (w *World) patchIndex3(px, py, pz int) int {
	return ((pz-w.MinPzcor)*w.height+(py-w.MinPycor))*w.width + (px - w.MinPxcor)
}

func (w *World) inBounds(px, py int) bool {
	return px >= w.MinPxcor && px <= w.MaxPxcor && py >= w.MinPycor && py <= w.MaxPycor
}

// MaxWorldSide is the largest number of patches on one axis.
// 100×100 is the size the web view is built for; 256 keeps a frame
// small enough to stream while go is running.
const MaxWorldSide = 256

// ValidWorld reports whether a resize-world size contains the origin
// and stays within MaxWorldSide. max >= min is implied when the origin
// is inside the rectangle.
func ValidWorld(minPx, maxPx, minPy, maxPy int) error {
	if minPx > 0 || maxPx < 0 || minPy > 0 || maxPy < 0 {
		return fmt.Errorf("the world must contain the origin")
	}
	if maxPx < minPx || maxPy < minPy {
		return fmt.Errorf("the world is empty")
	}
	if maxPx-minPx+1 > MaxWorldSide || maxPy-minPy+1 > MaxWorldSide {
		return fmt.Errorf("a world can be at most %d patches on a side", MaxWorldSide)
	}
	return nil
}

func (w *World) findLinkBreed(a, b *Turtle, breed string, directed bool) *Link {
	for _, l := range w.links {
		if l.dead || l.Directed != directed || l.Breed != breed {
			continue
		}
		if directed {
			if l.End1 == a && l.End2 == b {
				return l
			}
		} else if (l.End1 == a && l.End2 == b) || (l.End1 == b && l.End2 == a) {
			return l
		}
	}
	return nil
}

// ApplyView resizes the world and sets patch size and wrapping.
// Turtles and links die and patches are reset, as with resize-world.
// Patch size must be positive and at most 100.
func (w *World) ApplyView(minPx, maxPx, minPy, maxPy int, patchSize float64, wrapX, wrapY bool) error {
	if w.threeD {
		if err := ValidWorld3D(minPx, maxPx, minPy, maxPy, w.MinPzcor, w.MaxPzcor); err != nil {
			return err
		}
	} else if err := ValidWorld(minPx, maxPx, minPy, maxPy); err != nil {
		return err
	}
	if patchSize <= 0 || patchSize > 100 {
		return fmt.Errorf("patch size must be positive and at most 100")
	}
	w.mu.Lock()
	defer w.unlockPublish()
	w.Resize(minPx, maxPx, minPy, maxPy, wrapX, wrapY)
	w.PatchSize = patchSize
	return nil
}

// unlockPublish marks the published frame stale and releases mu.
// The caller must hold mu. The copy happens in CurrentView, so a headless
// run and a batch of steps inside one animation frame do not each clone
// the grid.
func (w *World) unlockPublish() {
	defer w.mu.Unlock()
	w.dirty.Store(true)
}

// Resize rebuilds the patch grid and removes every turtle and link.
// Patch variables and colors go back to their defaults. The who counter
// is left alone; clear-turtles and clear-all are what reset it.
func (w *World) Resize(minPx, maxPx, minPy, maxPy int, wrapX, wrapY bool) {
	if w.threeD {
		// NetLogo 3D worlds wrap on every axis.
		wrapX, wrapY = true, true
	}
	w.MinPxcor, w.MaxPxcor = minPx, maxPx
	w.MinPycor, w.MaxPycor = minPy, maxPy
	w.WrapX, w.WrapY = wrapX, wrapY
	w.turtles = map[int]*Turtle{}
	w.whoSeq = nil
	w.breedLive = nil
	w.links = nil
	w.recreatePatches()
}

// Seed reseeds the world RNG the way random-seed does in NetLogo:
// MersenneTwisterFast.setSeed, which uses only the low 32 bits.
func (w *World) Seed(seed int64) {
	w.seed = seed
	w.ResetRNGDraws()
	if w.rng == nil {
		w.rng = &MersenneTwister{draws: &w.rngDraws}
	}
	w.rng.SetSeed(seed)
}

// RNGSeed is the integer last passed to Seed. The caller serializes it
// with Seed; the world lock is not taken here.
func (w *World) RNGSeed() int64 {
	if w == nil {
		return 0
	}
	return w.seed
}

// RNG is the world's NetLogo-compatible random number generator.
func (w *World) RNG() *MersenneTwister { return w.rng }

// RNGDraws is how many 32-bit words the world RNG has produced since the
// last clear-all, setup, or random-seed. random 100 is two words (NetLogo
// uses nextLong), random-float is two, and a rejection inside nextInt or
// nextLong counts every word it read.
func (w *World) RNGDraws() uint64 {
	if w == nil {
		return 0
	}
	return w.rngDraws.Load()
}

// ResetRNGDraws zeroes the draw counter without changing the sequence.
func (w *World) ResetRNGDraws() {
	if w != nil {
		w.rngDraws.Store(0)
	}
}

// nextInt is MersenneTwisterFast.nextInt(n) for an int bound.
func (w *World) nextInt(n int) int {
	return int(w.rng.NextIntN(int32(n)))
}

// SetProgramVars installs the variable declarations from a compiled program.
// Existing global values are kept when the name survives.
func (w *World) SetProgramVars(p *Program) {
	w.turtlesOwn = append([]string(nil), p.TurtlesOwn...)
	w.patchesOwn = append([]string(nil), p.PatchesOwn...)
	w.linksOwn = append([]string(nil), p.LinksOwn...)
	w.breedOwn = map[string][]string{}
	for k, v := range p.BreedOwn {
		w.breedOwn[k] = append([]string(nil), v...)
	}
	w.breeds = map[string]*breedDef{}
	for _, b := range p.Breeds {
		w.breeds[b.Plural] = &breedDef{Plural: b.Plural, Singular: b.Singular, Own: append([]string(nil), p.BreedOwn[b.Plural]...)}
	}
	w.linkBreeds = map[string]*linkBreedDef{}
	for _, b := range p.LinkBreeds {
		w.linkBreeds[b.Plural] = &linkBreedDef{Plural: b.Plural, Singular: b.Singular, Directed: b.Directed, Own: append([]string(nil), p.LinkBreedOwn[b.Plural]...)}
	}
	w.initBreedCalls(p)
	w.ifaceGlob = map[string]bool{}
	for name := range p.InterfaceVars {
		w.ifaceGlob[name] = true
		if _, ok := w.globals[name]; !ok {
			w.globals[name] = Num(0)
		}
	}
	for _, name := range p.Globals {
		if _, ok := w.globals[name]; !ok {
			w.globals[name] = Num(0)
		}
	}
	// Make sure every patch has the declared variables.
	for _, p := range w.patches {
		if p.Vars == nil {
			p.Vars = map[string]Value{}
		}
		for _, name := range w.patchesOwn {
			if _, ok := p.Vars[name]; !ok {
				p.Vars[name] = Num(0)
			}
		}
	}
}

func (w *World) SetGlobal(name string, v Value) {
	w.mu.Lock()
	w.globals[name] = v
	w.mu.Unlock()
}
func (w *World) Global(name string) (Value, bool) {
	v, ok := w.globals[name]
	return v, ok
}
func (w *World) Globals() map[string]Value { return w.globals }

func (w *World) initBreedCalls(p *Program) {
	w.breedCalls = make(map[string]breedCallDef)
	if p == nil {
		return
	}
	for _, b := range p.LinkBreeds {
		names := []string{b.Plural}
		if b.Singular != "" && b.Singular != b.Plural {
			names = append(names, b.Singular)
		}
		for _, name := range names {
			w.breedCalls[name+"-neighbor?"] = breedCallDef{kind: breedCallLinkNeighbor, breed: b.Plural, dir: 0}
			w.breedCalls["in-"+name+"-neighbor?"] = breedCallDef{kind: breedCallLinkNeighbor, breed: b.Plural, dir: -1}
			w.breedCalls["out-"+name+"-neighbor?"] = breedCallDef{kind: breedCallLinkNeighbor, breed: b.Plural, dir: 1}
		}
		if b.Singular != "" {
			w.breedCalls[b.Singular] = breedCallDef{kind: breedCallLinkSingular, breed: b.Plural, directed: b.Directed}
			if b.Directed {
				w.breedCalls["in-"+b.Singular+"-from"] = breedCallDef{kind: breedCallInLinkFrom, breed: b.Plural, directed: true}
				w.breedCalls["out-"+b.Singular+"-to"] = breedCallDef{kind: breedCallOutLinkTo, breed: b.Plural, directed: true}
			} else {
				w.breedCalls[b.Singular+"-with"] = breedCallDef{kind: breedCallLinkWith, breed: b.Plural, directed: false}
			}
		}
		w.breedCalls["my-"+b.Plural] = breedCallDef{kind: breedCallMyLinks, breed: b.Plural, dir: 0}
		if b.Directed {
			w.breedCalls["my-in-"+b.Plural] = breedCallDef{kind: breedCallMyLinks, breed: b.Plural, dir: -1}
			w.breedCalls["my-out-"+b.Plural] = breedCallDef{kind: breedCallMyLinks, breed: b.Plural, dir: 1}
		}
	}
	for _, b := range p.Breeds {
		if b.Singular != "" {
			w.breedCalls["is-"+b.Singular+"?"] = breedCallDef{kind: breedCallIsBreed, breed: b.Plural}
			w.breedCalls[b.Singular] = breedCallDef{kind: breedCallSingular, breed: b.Plural}
		}
		if b.Plural != "" {
			w.breedCalls[b.Plural+"-on"] = breedCallDef{kind: breedCallOn, breed: b.Plural}
			w.breedCalls[b.Plural+"-at"] = breedCallDef{kind: breedCallAt, breed: b.Plural}
		}
	}
}

func (w *World) Breed(plural string) (*breedDef, bool) {
	b, ok := w.breeds[plural]
	return b, ok
}

func (w *World) LinkBreed(plural string) (*linkBreedDef, bool) {
	b, ok := w.linkBreeds[plural]
	return b, ok
}

func (w *World) SetDefaultShape(breedPlural, shape string) {
	if breedPlural == "" {
		breedPlural = "turtles"
	}
	w.defaultShape[breedPlural] = shape
}

func (w *World) defaultShapeFor(breed string) string {
	if breed != "" {
		if s, ok := w.defaultShape[breed]; ok {
			return s
		}
	}
	if s, ok := w.defaultShape["turtles"]; ok {
		return s
	}
	return "default"
}

// --- topology ---

func (w *World) worldWidth() float64  { return float64(w.width) }
func (w *World) worldHeight() float64 { return float64(w.height) }

// Width and Height are the patch counts of the grid.
func (w *World) Width() int  { return w.width }
func (w *World) Height() int { return w.height }

// wrapDouble wraps pos into [min, max). NetLogo's Topology.wrap.
func wrapDouble(pos, min, max float64) float64 {
	if pos >= max {
		return min + math.Mod(pos-max, max-min)
	}
	if pos < min {
		result := max - math.Mod(min-pos, max-min)
		if result < max {
			return result
		}
		return min
	}
	return pos
}

func wrapPcor(pos, min, max int) int {
	span := 1 + max - min
	if pos > max {
		return min + ((pos - min) % span)
	}
	if pos < min {
		// NetLogo assumes min <= 0 because the world contains the origin.
		return ((pos-max)%span+span)%span + min
		// The Scala source uses ((pos - max) % span) + max, which in Java
		// truncates toward zero. Replicate that.
	}
	return pos
}

func wrapPcorJava(pos, min, max int) int {
	span := 1 + max - min
	if pos > max {
		return min + modIntTrunc(pos-min, span)
	}
	if pos < min {
		return modIntTrunc(pos-max, span) + max
	}
	return pos
}

func modIntTrunc(a, b int) int {
	// Java % truncates toward zero. Go % does too for integers.
	return a % b
}

// wrapCoordX wraps a turtle x coordinate, or reports that it left the world.
// The legal range on an axis is [minPcor-0.5, maxPcor+0.5).
func (w *World) wrapCoordX(x float64) (float64, error) {
	min := float64(w.MinPxcor) - 0.5
	max := float64(w.MaxPxcor) + 0.5
	if w.WrapX {
		return wrapDouble(x, min, max), nil
	}
	if x >= max || x < min {
		return x, errOutsideWorld
	}
	return x, nil
}

func (w *World) wrapCoordY(y float64) (float64, error) {
	min := float64(w.MinPycor) - 0.5
	max := float64(w.MaxPycor) + 0.5
	if w.WrapY {
		return wrapDouble(y, min, max), nil
	}
	if y >= max || y < min {
		return y, errOutsideWorld
	}
	return y, nil
}

// errOutsideWorld is the internal signal that a coordinate left a
// non-wrapping axis. Commands decide whether that is a runtime error.
var errOutsideWorld = fmt.Errorf("the turtle cannot move beyond the world's edge")

// errTiedOutside means a tied follower could not follow. fd swallows the
// root's own boundary and stops; it does not swallow this.
var errTiedOutside = fmt.Errorf("a tied turtle cannot move beyond the world's edge")

// edgeError is a root turtle leaving the world. Unwrap exposes
// errOutsideWorld so fd can stop quietly without hiding errTiedOutside.
type edgeError struct{ msg string }

func (e *edgeError) Error() string { return e.msg }
func (e *edgeError) Unwrap() error { return errOutsideWorld }

func outsidePoint(x, y float64) error {
	return &edgeError{msg: fmt.Sprintf("the point [%s %s] is outside of the world", formatNum(x), formatNum(y))}
}

func (w *World) roundPatch(v float64) int {
	return int(math.Floor(v + 0.5))
}

// PatchAt returns the patch containing the point, or nil if it is outside
// a non-wrapping world.
func (w *World) PatchAt(x, y float64) *Patch {
	var err error
	if w.WrapX {
		x = wrapDouble(x, float64(w.MinPxcor)-0.5, float64(w.MaxPxcor)+0.5)
	}
	if w.WrapY {
		y = wrapDouble(y, float64(w.MinPycor)-0.5, float64(w.MaxPycor)+0.5)
	}
	px := w.roundPatch(x)
	py := w.roundPatch(y)
	if !w.inBounds(px, py) {
		return nil
	}
	_ = err
	return w.patches[w.patchIndex(px, py)]
}

// PatchAtPcor returns the patch at integer coordinates, wrapping if allowed.
// Nil if the coordinate is outside a non-wrapping world.
func (w *World) PatchAtPcor(px, py int) *Patch {
	if w.WrapX {
		px = wrapPcorJava(px, w.MinPxcor, w.MaxPxcor)
	}
	if w.WrapY {
		py = wrapPcorJava(py, w.MinPycor, w.MaxPycor)
	}
	if !w.inBounds(px, py) {
		return nil
	}
	return w.patches[w.patchIndex(px, py)]
}

func (w *World) GetPatch(px, py int) *Patch {
	if !w.inBounds(px, py) {
		return nil
	}
	return w.patches[w.patchIndex(px, py)]
}

// --- clearing ---

func (w *World) ClearAll() {
	// setup calls clear-all. The draw counter starts over so a run's
	// rng_calls figure is the draws since this reset, not since process start.
	w.ResetRNGDraws()
	w.ClearGlobals()
	w.ClearTicks()
	w.ClearTurtles()
	w.ClearLinks()
	w.ClearPatches()
	w.ClearDrawing()
	w.ResetPerspective()
}

func (w *World) ClearGlobals() {
	for name := range w.globals {
		if w.ifaceGlob[name] {
			continue
		}
		w.globals[name] = Num(0)
	}
}

func (w *World) ClearTicks() {
	w.ticks = 0
	w.ticksStarted = false
	w.runEpoch++
}

func (w *World) ResetTicks() {
	w.ticks = 0
	w.ticksStarted = true
	w.runEpoch++
}

// RunEpoch changes when ticks are cleared or reset. It does not change
// when the model ticks.
func (w *World) RunEpoch() uint64 { return w.runEpoch }

func (w *World) Tick() error {
	if !w.ticksStarted {
		return fmt.Errorf("the tick counter has not been started yet. Use RESET-TICKS")
	}
	w.ticks++
	return nil
}

func (w *World) TickAdvance(n float64) error {
	if !w.ticksStarted {
		return fmt.Errorf("the tick counter has not been started yet. Use RESET-TICKS")
	}
	w.ticks += n
	return nil
}

func (w *World) Ticks() (float64, error) {
	if !w.ticksStarted {
		return 0, fmt.Errorf("the tick counter has not been started yet. Use RESET-TICKS")
	}
	return w.ticks, nil
}

func (w *World) TicksStarted() bool { return w.ticksStarted }

func (w *World) ClearPatches() {
	for _, p := range w.patches {
		p.PColor = 0
		p.HasRGB = false
		p.Label = Str("")
		p.turtles = p.turtles[:0]
		for _, name := range w.patchesOwn {
			p.Vars[name] = Num(0)
		}
		// drop unknown keys that aren't declared? keep only declared
		if len(p.Vars) != len(w.patchesOwn) {
			nv := map[string]Value{}
			for _, name := range w.patchesOwn {
				nv[name] = Num(0)
			}
			p.Vars = nv
		}
	}
	// turtles were removed from patches; if turtles still exist they must
	// reattach. ClearPatches in NetLogo does not remove turtles, but it
	// does reset patches. Reattach live turtles.
	for _, t := range w.liveTurtles() {
		t.patch = nil
		w.attach(t)
	}
}

// ClearTurtles kills every turtle and link and resets the who counter, so
// the next turtle created is turtle 0, as NetLogo's clear-turtles does.
// clear-all calls it. die does not reset the counter.
func (w *World) ClearTurtles() {
	w.nextWho = 0
	for _, t := range w.turtles {
		t.dead = true
		t.patch = nil
	}
	w.turtles = map[int]*Turtle{}
	w.whoSeq = nil
	w.breedLive = nil
	for _, p := range w.patches {
		p.turtles = p.turtles[:0]
	}
	// links die with their turtles
	for _, l := range w.links {
		l.dead = true
	}
	w.links = nil
}

func (w *World) ClearLinks() {
	for _, l := range w.links {
		l.dead = true
	}
	w.links = nil
}

// --- turtles ---

func (w *World) liveTurtles() []*Turtle {
	out := make([]*Turtle, 0, len(w.turtles))
	// who order
	if len(w.whoSeq) != len(w.turtles) {
		w.rebuildWhoSeq()
	}
	for _, id := range w.whoSeq {
		t := w.turtles[id]
		if t != nil && !t.dead {
			out = append(out, t)
		}
	}
	return out
}

func (w *World) rebuildWhoSeq() {
	w.whoSeq = w.whoSeq[:0]
	for id, t := range w.turtles {
		if t != nil && !t.dead {
			w.whoSeq = append(w.whoSeq, id)
		}
	}
	// insertion sort, n is modest
	sort.Ints(w.whoSeq)
}

// dropWho removes one who from the ordered id list. Creating a turtle
// appends its who, which is always the new maximum, so the list stays
// sorted without a full rebuild on every death.
func (w *World) dropWho(id int) {
	seq := w.whoSeq
	i := sort.SearchInts(seq, id)
	if i < len(seq) && seq[i] == id {
		copy(seq[i:], seq[i+1:])
		w.whoSeq = seq[:len(seq)-1]
		return
	}
	w.rebuildWhoSeq()
}

func (w *World) Turtles() *AgentSet {
	live := w.liveTurtles()
	ags := make([]Agent, len(live))
	for i, t := range live {
		ags[i] = t
	}
	return &AgentSet{Kind: KindTurtle, All: true, Agents: ags}
}

// collectBreed appends living turtles of one breed in who order.
// The buffer is reused by ask so a breed ask does not allocate an agentset.
func (w *World) trackBreed(t *Turtle) {
	if t == nil || t.Breed == "" {
		return
	}
	if w.breedLive == nil {
		w.breedLive = map[string][]*Turtle{}
	}
	w.breedLive[t.Breed] = append(w.breedLive[t.Breed], t)
}

func (w *World) untrackBreed(t *Turtle) {
	if t == nil || t.Breed == "" || w.breedLive == nil {
		return
	}
	s := w.breedLive[t.Breed]
	for i, o := range s {
		if o == t {
			copy(s[i:], s[i+1:])
			s[len(s)-1] = nil
			w.breedLive[t.Breed] = s[:len(s)-1]
			return
		}
	}
}

func (w *World) collectBreed(plural string, buf []Agent) []Agent {
	buf = buf[:0]
	if w.breedLive != nil {
		for _, t := range w.breedLive[plural] {
			if t != nil && !t.dead && t.Breed == plural {
				buf = append(buf, t)
			}
		}
		if len(buf) > 1 {
			sort.Slice(buf, func(i, j int) bool {
				return buf[i].(*Turtle).Who < buf[j].(*Turtle).Who
			})
		}
		return buf
	}
	if len(w.whoSeq) != len(w.turtles) {
		w.rebuildWhoSeq()
	}
	for _, id := range w.whoSeq {
		t := w.turtles[id]
		if t != nil && !t.dead && t.Breed == plural {
			buf = append(buf, t)
		}
	}
	return buf
}

// anyBreedOn reports whether a living turtle of breed is standing on p.
// Nil patch is false, matching cans-on nobody.
func (w *World) anyBreedOn(p *Patch, breed string) bool {
	if p == nil {
		return false
	}
	for _, t := range p.turtles {
		if t != nil && !t.dead && t.Breed == breed {
			return true
		}
	}
	return false
}

func (w *World) TurtlesOfBreed(plural string) *AgentSet {
	live := w.liveTurtles()
	ags := make([]Agent, 0, len(live))
	for _, t := range live {
		if t.Breed == plural {
			ags = append(ags, t)
		}
	}
	return &AgentSet{Kind: KindTurtle, Breed: plural, Named: true, Agents: ags}
}

func (w *World) Turtle(who int) *Turtle {
	t := w.turtles[who]
	if t == nil || t.dead {
		return nil
	}
	return t
}

func (w *World) Patches() *AgentSet {
	if w.patchSet != nil {
		return w.patchSet
	}
	ags := make([]Agent, len(w.patchOrder))
	for i, p := range w.patchOrder {
		ags[i] = p
	}
	w.patchSet = &AgentSet{Kind: KindPatch, All: true, Agents: ags}
	return w.patchSet
}

func (w *World) initTurtleVars(t *Turtle) {
	n := len(w.turtlesOwn)
	if t.Breed != "" {
		n += len(w.breedOwn[t.Breed])
	}
	t.Vars = make(map[string]Value, n)
	for _, name := range w.turtlesOwn {
		t.Vars[name] = Num(0)
	}
	if t.Breed != "" {
		for _, name := range w.breedOwn[t.Breed] {
			t.Vars[name] = Num(0)
		}
	}
}

func (w *World) attach(t *Turtle) {
	p := w.PatchAt3(t.X, t.Y, t.Z)
	t.patch = p
	if p != nil {
		p.turtles = append(p.turtles, t)
	}
}

func (w *World) detach(t *Turtle) {
	p := t.patch
	if p == nil {
		return
	}
	ts := p.turtles
	for i, o := range ts {
		if o == t {
			copy(ts[i:], ts[i+1:])
			ts[len(ts)-1] = nil
			p.turtles = ts[:len(ts)-1]
			break
		}
	}
	t.patch = nil
}

// CreateTurtle creates one turtle. colorN in 0..13 selects a base hue
// (5+10*colorN). heading is in degrees. Position is (x, y).
func (w *World) CreateTurtle(breed string, x, y float64, colorN, heading int) *Turtle {
	return w.CreateTurtle3(breed, x, y, 0, colorN, heading)
}

// CreateTurtle3 is CreateTurtle with a z coordinate. Pitch and roll start
// at 0, as for NetLogo 3D's create-turtles and sprout. z must be 0 in a
// 2D world.
func (w *World) CreateTurtle3(breed string, x, y, z float64, colorN, heading int) *Turtle {
	t := &Turtle{
		Who:     w.nextWho,
		X:       x,
		Y:       y,
		Z:       z,
		Heading: float64(heading),
		Color:   float64(5 + 10*colorN),
		Shape:   w.defaultShapeFor(breed),
		Size:    1,
		Breed:   breed,
		Label:   Str(""),
		PenSize: 1,
		world:   w,
	}
	w.normalizeHeading(t)
	w.initTurtleVars(t)
	w.nextWho++
	w.turtles[t.Who] = t
	w.whoSeq = append(w.whoSeq, t.Who)
	w.trackBreed(t)
	w.attach(t)
	return t
}

func (w *World) normalizeHeading(t *Turtle) {
	h := math.Mod(t.Heading, 360)
	if h < 0 {
		h += 360
	}
	t.Heading = h
}

// Hatch copies parent. breed overrides the breed when non-empty.
func (w *World) Hatch(parent *Turtle, breed string) *Turtle {
	b := parent.Breed
	if breed != "" {
		b = breed
	}
	t := &Turtle{
		Who:      w.nextWho,
		X:        parent.X,
		Y:        parent.Y,
		Z:        parent.Z,
		Heading:  parent.Heading,
		Pitch:    parent.Pitch,
		Roll:     parent.Roll,
		Color:    parent.Color,
		RGB:      parent.RGB,
		HasRGB:   parent.HasRGB,
		Shape:    parent.Shape,
		Size:     parent.Size,
		Hidden:   parent.Hidden,
		Label:    parent.Label,
		Breed:    b,
		PenDown:  parent.PenDown,
		PenErase: parent.PenErase,
		PenSize:  parent.PenSize,
		world:    w,
	}
	t.Vars = make(map[string]Value, len(parent.Vars))
	for k, v := range parent.Vars {
		t.Vars[k] = v
	}
	// breed-own vars that the parent didn't have
	if b != "" {
		for _, name := range w.breedOwn[b] {
			if _, ok := t.Vars[name]; !ok {
				t.Vars[name] = Num(0)
			}
		}
	}
	w.nextWho++
	w.turtles[t.Who] = t
	w.whoSeq = append(w.whoSeq, t.Who)
	w.trackBreed(t)
	w.attach(t)
	return t
}

// Die removes a turtle or link. Patch die is an error at the call site.
func (w *World) Die(a Agent) {
	switch t := a.(type) {
	case *Turtle:
		if t.dead {
			return
		}
		t.dead = true
		w.untrackBreed(t)
		w.detach(t)
		delete(w.turtles, t.Who)
		// remove links
		kept := w.links[:0]
		for _, l := range w.links {
			if l.dead {
				continue
			}
			if l.End1 == t || l.End2 == t {
				l.dead = true
				continue
			}
			kept = append(kept, l)
		}
		w.links = kept
		w.dropWho(t.Who)
	case *Link:
		if t.dead {
			return
		}
		t.dead = true
		kept := w.links[:0]
		for _, l := range w.links {
			if l != t && !l.dead {
				kept = append(kept, l)
			}
		}
		w.links = kept
	}
	if w.perspectiveAgent == a {
		w.ResetPerspective()
	}
}

func (w *World) SetPerspective(mode string, a Agent) {
	w.perspectiveMode = mode
	w.perspectiveAgent = a
}

func (w *World) ResetPerspective() {
	w.perspectiveMode = ""
	w.perspectiveAgent = nil
}

func (w *World) Perspective() (string, Agent) {
	if w.perspectiveAgent != nil && w.perspectiveAgent.IsDead() {
		w.ResetPerspective()
	}
	return w.perspectiveMode, w.perspectiveAgent
}

// MoveTo sets a turtle's position. A wrapping axis wraps. A coordinate
// outside a non-wrapping axis is a runtime error and the turtle stays put.
// In a 3D world the turtle keeps its zcor, as setxy does in NetLogo 3D.
func (w *World) MoveTo(t *Turtle, x, y float64) error {
	return w.moveToInternal(t, x, y, t.Z, make(map[*Turtle]bool))
}

// MoveTo3 sets all three coordinates. In a 2D world z is ignored.
func (w *World) MoveTo3(t *Turtle, x, y, z float64) error {
	if !w.threeD {
		z = 0
	}
	return w.moveToInternal(t, x, y, z, make(map[*Turtle]bool))
}

// tiedOther is the turtle that follows root across this link.
// A directed link follows only from end1 to end2. An undirected link
// follows in either direction. tie-mode "none" does not follow.
func tiedOther(root *Turtle, l *Link) *Turtle {
	if l == nil || l.dead || root == nil || l.TieMode == "none" || l.TieMode == "" {
		return nil
	}
	if l.Directed {
		if l.End1 == root {
			return l.End2
		}
		return nil
	}
	if l.End1 == root {
		return l.End2
	}
	if l.End2 == root {
		return l.End1
	}
	return nil
}

func tieMoveErr(err error) error {
	if err == nil || errors.Is(err, errTiedOutside) {
		return err
	}
	if errors.Is(err, errOutsideWorld) {
		return errTiedOutside
	}
	return err
}

func (w *World) moveToInternal(t *Turtle, x, y, z float64, visited map[*Turtle]bool) error {
	if visited[t] {
		return nil
	}
	visited[t] = true
	return w.moveToMarked(t, x, y, z, visited)
}

// moveToMarked moves t, which is already in visited, and translates turtles
// tied to it that have not been visited yet. z is used only in a 3D world.
func (w *World) moveToMarked(t *Turtle, x, y, z float64, visited map[*Turtle]bool) error {
	nx, errX := w.wrapCoordX(x)
	ny, errY := w.wrapCoordY(y)
	nz, errZ := w.wrapCoordZ(z)
	if errX != nil || errY != nil || errZ != nil {
		if w.threeD {
			return outsidePoint3(x, y, z)
		}
		return outsidePoint(x, y)
	}
	oldX, oldY, oldZ := t.X, t.Y, t.Z
	dx := nx - oldX
	dy := ny - oldY
	dz := nz - oldZ

	if t.PenDown || t.PenErase {
		w.drawLine(oldX, oldY, oldZ, nx, ny, nz, t.Color, t.PenSize, t.PenErase)
	}
	// Like NetLogo, a turtle changes its patch's turtle list only when it
	// crosses into another patch, so turtles-here keeps arrival order.
	if np := w.PatchAt3(nx, ny, nz); np != t.patch || np == nil {
		w.detach(t)
		t.X, t.Y, t.Z = nx, ny, nz
		w.attach(t)
	} else {
		t.X, t.Y, t.Z = nx, ny, nz
	}

	var tiedErr error
	for _, l := range t.links {
		leaf := tiedOther(t, l)
		if leaf == nil || leaf.dead || visited[leaf] {
			continue
		}
		if err := w.moveToInternal(leaf, leaf.X+dx, leaf.Y+dy, leaf.Z+dz, visited); err != nil && tiedErr == nil {
			tiedErr = tieMoveErr(err)
		}
	}
	return tiedErr
}

// fdInfinitesimal matches NetLogo's Numbers.Infinitesimal. A leftover
// smaller than this is treated as already arrived.
const fdInfinitesimal = 3.2e-15

// Forward moves one unit at a time, like fd and bk. When the next step
// would leave a non-wrapping world, the turtle stops at the last position
// that was still inside. The distance may be negative, which moves backward.
func (w *World) Forward(t *Turtle, dist float64) error {
	for {
		mag := math.Abs(dist)
		if mag <= fdInfinitesimal {
			return nil
		}
		if mag <= 1 {
			return swallowEdge(w.Jump(t, dist))
		}
		step := 1.0
		if dist < 0 {
			step = -1
		}
		if err := w.Jump(t, step); err != nil {
			return swallowEdge(err)
		}
		dist -= step
	}
}

// swallowEdge turns the moving turtle's own out-of-world error into a
// quiet stop. A tied follower that cannot follow is still reported.
func swallowEdge(err error) error {
	if err == nil || errors.Is(err, errTiedOutside) {
		return err
	}
	if errors.Is(err, errOutsideWorld) {
		return nil
	}
	return err
}

// Jump moves the whole distance in one step. If that point is outside a
// non-wrapping world, the turtle does not move and the error is returned.
// fd catches the error and stops; the jump command ignores it.
func (w *World) Jump(t *Turtle, dist float64) error {
	if w.threeD {
		dx, dy, dz := orientDelta(t.Heading, t.Pitch, dist)
		return w.MoveTo3(t, t.X+dx, t.Y+dy, t.Z+dz)
	}
	dx, dy := headingDelta(t.Heading, dist)
	return w.MoveTo(t, t.X+dx, t.Y+dy)
}

func headingDelta(heading, dist float64) (float64, float64) {
	rad := heading * math.Pi / 180
	s, c := math.Sincos(rad)
	return dist * s, dist * c
}

// CanMove reports whether a single jump of dist would stay in the world.
// It matches patch-ahead dist != nobody, including a negative distance.
func (w *World) CanMove(t *Turtle, dist float64) bool {
	if w.threeD {
		dx, dy, dz := orientDelta(t.Heading, t.Pitch, dist)
		return w.PatchAt3(t.X+dx, t.Y+dy, t.Z+dz) != nil
	}
	dx, dy := headingDelta(t.Heading, dist)
	return w.PatchAt(t.X+dx, t.Y+dy) != nil
}

func (w *World) SetHeading(t *Turtle, h float64) error {
	delta := h - t.Heading
	return w.Turn(t, delta)
}

func (w *World) Turn(t *Turtle, delta float64) error {
	return w.turnInternal(t, delta, make(map[*Turtle]bool))
}

func (w *World) turnInternal(t *Turtle, delta float64, visited map[*Turtle]bool) error {
	if visited[t] {
		return nil
	}
	visited[t] = true

	t.Heading += delta
	w.normalizeHeading(t)

	rad := delta * math.Pi / 180
	cosA, sinA := math.Cos(rad), math.Sin(rad)

	// Mark every direct follower before moving any of them. A turtle
	// reached by two ties is then rotated once, and moveToMarked will
	// not drag it along as someone else's follower.
	type turned struct {
		leaf *Turtle
		mode string
		x, y float64
	}
	var leaves []turned
	for _, l := range t.links {
		leaf := tiedOther(t, l)
		if leaf == nil || leaf.dead || visited[leaf] {
			continue
		}
		visited[leaf] = true
		relX := leaf.X - t.X
		relY := leaf.Y - t.Y
		leaves = append(leaves, turned{
			leaf: leaf,
			mode: l.TieMode,
			x:    t.X + relX*cosA + relY*sinA,
			y:    t.Y - relX*sinA + relY*cosA,
		})
	}
	var tiedErr error
	for _, lf := range leaves {
		err := w.moveToMarked(lf.leaf, lf.x, lf.y, lf.leaf.Z, visited)
		if err != nil {
			if tiedErr == nil {
				tiedErr = tieMoveErr(err)
			}
			// A follower that cannot move keeps its heading.
			continue
		}
		if lf.mode == "fixed" {
			lf.leaf.Heading += delta
			w.normalizeHeading(lf.leaf)
		}
	}
	return tiedErr
}

// PatchHere is the patch under a turtle.
func (w *World) PatchHere(t *Turtle) *Patch {
	if t.patch != nil && !t.patch.dead {
		return t.patch
	}
	return w.PatchAt3(t.X, t.Y, t.Z)
}

func (w *World) TurtlesHere(p *Patch) []*Turtle {
	if p == nil {
		return nil
	}
	out := make([]*Turtle, 0, len(p.turtles))
	for _, t := range p.turtles {
		if t != nil && !t.dead {
			out = append(out, t)
		}
	}
	// Arrival order, as NetLogo's Patch.turtlesHere keeps it. one-of and
	// n-of index into this order, so sorting it would change which agent
	// a seeded run picks.
	return out
}

// Neighbors returns the 8 surrounding patches (fewer at the edge of a box).
// neighborRing8 is NetLogo's neighbors order (Neighbors.getNeighbors):
// N, E, S, W, NE, SE, SW, NW. one-of neighbors, uphill, and ask
// neighbors index this order, so it must match for seeded runs.
var neighborRing8 = [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}, {1, 1}, {1, -1}, {-1, -1}, {-1, 1}}

func (w *World) Neighbors(p *Patch) []*Patch {
	return w.neighbors(p, false)
}

func (w *World) Neighbors4(p *Patch) []*Patch {
	return w.neighbors(p, true)
}

func (w *World) neighbors(p *Patch, four bool) []*Patch {
	if w.threeD {
		// neighbors is the 26 surrounding cells. neighbors4 stays the four
		// side neighbors on the patch's own layer, as in NetLogo 3D.
		if four {
			return w.neighborsAt(p, neighborOffs4)
		}
		return w.neighborsAt(p, neighborOffs26)
	}
	var offs [][2]int
	if four {
		offs = [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
	} else {
		offs = neighborRing8
	}
	out := make([]*Patch, 0, len(offs))
	for _, o := range offs {
		np := w.PatchAtPcor(p.PX+o[0], p.PY+o[1])
		// A wrapping world only one patch wide folds a neighbor back onto
		// the source. NetLogo drops that patch, and drops duplicates when
		// east and west (or north and south) are the same patch.
		if np == nil || np == p || patchListed(out, np) {
			continue
		}
		out = append(out, np)
	}
	return out
}

func patchListed(ps []*Patch, p *Patch) bool {
	for _, o := range ps {
		if o == p {
			return true
		}
	}
	return false
}

// ShortestDX is the signed offset from x1 to x2. A wrapping axis uses the
// shorter arc, with the halfway tie broken toward the negative direction,
// which is NetLogo's [-size/2, size/2) range. A non-wrapping axis keeps
// the straight difference, even when the other way around would be shorter.
func (w *World) ShortestDX(x1, x2 float64) float64 {
	d := x2 - x1
	if !w.WrapX {
		return d
	}
	return shortestWrap(d, w.worldWidth())
}

func (w *World) ShortestDY(y1, y2 float64) float64 {
	d := y2 - y1
	if !w.WrapY {
		return d
	}
	return shortestWrap(d, w.worldHeight())
}

func shortestWrap(d, size float64) float64 {
	if d >= size/2 {
		d -= size
	} else if d < -size/2 {
		d += size
	}
	return d
}

func (w *World) Distance(x1, y1, x2, y2 float64) float64 {
	dx := w.ShortestDX(x1, x2)
	dy := w.ShortestDY(y1, y2)
	return math.Hypot(dx, dy)
}

// HeadingTowards returns the heading from (x1,y1) to (x2,y2).
// 0 is north, 90 is east. The same point has no heading.
func (w *World) HeadingTowards(x1, y1, x2, y2 float64) (float64, error) {
	dx := w.ShortestDX(x1, x2)
	dy := w.ShortestDY(y1, y2)
	if dx == 0 && dy == 0 {
		return 0, fmt.Errorf("no heading is defined from a point to itself")
	}
	h := math.Atan2(dx, dy) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h, nil
}

// RandomXcor is random-xcor: min + nextDouble * (max - min) with the
// patch-edge bounds, as NetLogo computes it.
func (w *World) RandomXcor() float64 {
	lo := float64(w.MinPxcor) - 0.5
	hi := float64(w.MaxPxcor) + 0.5
	return lo + float64(w.rng.NextDouble()*(hi-lo))
}

// RandomYcor is random-ycor.
func (w *World) RandomYcor() float64 {
	lo := float64(w.MinPycor) - 0.5
	hi := float64(w.MaxPycor) + 0.5
	return lo + float64(w.rng.NextDouble()*(hi-lo))
}

// RandomPxcor is random-pxcor: min + nextInt(max - min + 1).
func (w *World) RandomPxcor() int {
	return w.MinPxcor + w.nextInt(w.MaxPxcor-w.MinPxcor+1)
}

// RandomPycor is random-pycor.
func (w *World) RandomPycor() int {
	return w.MinPycor + w.nextInt(w.MaxPycor-w.MinPycor+1)
}

// RandomInt is random with an integer bound. n > 0 yields [0, n), n < 0
// yields (n, 0], and 0 yields 0. Like NetLogo it draws with nextLong.
func (w *World) RandomInt(n int) (int, error) {
	v, err := w.Random(float64(n))
	return int(v), err
}

// Random implements NetLogo's random reporter (_random). A non-integer
// bound is rounded away from zero first, so random 2.5 is random 3. The
// draw is nextLong(bound), not nextInt.
func (w *World) Random(max float64) (float64, error) {
	// validLong: 2^53 is the largest integer a double holds exactly.
	if max > 9007199254740992 || max < -9007199254740992 {
		return 0, fmt.Errorf("%s is too large to be represented exactly as an integer in NetLogo", formatNum(max))
	}
	if math.IsNaN(max) {
		return 0, nil // Java's (long) NaN is 0
	}
	n := int64(max)
	if max != float64(n) {
		if max >= 0 {
			n++
		} else {
			n--
		}
	}
	switch {
	case n > 0:
		return float64(w.rng.NextLongN(n)), nil
	case n < 0:
		return -float64(w.rng.NextLongN(-n)), nil
	}
	return 0, nil
}

// RandomFloat is random-float: n * nextDouble. It always draws, even for 0.
func (w *World) RandomFloat(n float64) (float64, error) {
	return n * w.rng.NextDouble(), nil
}

// --- links ---

func (w *World) Links() *AgentSet {
	ags := make([]Agent, 0, len(w.links))
	for _, l := range w.links {
		if l != nil && !l.dead {
			ags = append(ags, l)
		}
	}
	return &AgentSet{Kind: KindLink, All: true, Agents: ags}
}

func (w *World) findLink(a, b *Turtle, directed bool) *Link {
	for _, l := range w.links {
		if l.dead {
			continue
		}
		if l.Directed != directed {
			continue
		}
		if directed {
			if l.End1 == a && l.End2 == b {
				return l
			}
		} else if (l.End1 == a && l.End2 == b) || (l.End1 == b && l.End2 == a) {
			return l
		}
	}
	return nil
}

// CreateLink creates a link. directed selects create-link-to style.
// Returns the existing link if one is already there.
func (w *World) CreateLink(a, b *Turtle, directed bool, breed string) (*Link, bool, error) {
	if a == nil || b == nil || a.dead || b.dead {
		return nil, false, fmt.Errorf("a link cannot be created with a dead turtle")
	}
	if a == b {
		return nil, false, fmt.Errorf("a turtle cannot link to itself")
	}
	if existing := w.findLink(a, b, directed); existing != nil {
		return existing, false, nil
	}
	l := &Link{
		ID:        w.nextLID,
		End1:      a,
		End2:      b,
		Directed:  directed,
		Breed:     breed,
		Color:     5,
		Shape:     "default",
		Thickness: 0,
		TieMode:   "none",
		Label:     Str(""),
		Vars:      map[string]Value{},
		world:     w,
	}
	w.nextLID++
	for _, name := range w.linksOwn {
		l.Vars[name] = Num(0)
	}
	if breed != "" {
		for _, name := range w.linkBreeds[breed].ownOrEmpty() {
			l.Vars[name] = Num(0)
		}
	}
	w.links = append(w.links, l)
	a.links = append(a.links, l)
	b.links = append(b.links, l)
	return l, true, nil
}

func (b *linkBreedDef) ownOrEmpty() []string {
	if b == nil {
		return nil
	}
	return b.Own
}

func (w *World) LinksOf(t *Turtle) []*Link {
	if t == nil {
		return nil
	}
	kept := t.links[:0]
	for _, l := range t.links {
		if !l.dead && (l.End1 == t || l.End2 == t) {
			kept = append(kept, l)
		}
	}
	t.links = kept
	out := make([]*Link, len(kept))
	copy(out, kept)
	return out
}

func (w *World) LinkNeighbors(t *Turtle, dir int) []*Turtle {
	// dir: 0 either, 1 out, -1 in
	seen := map[*Turtle]bool{}
	var out []*Turtle
	for _, l := range w.LinksOf(t) {
		var other *Turtle
		switch dir {
		case 1:
			if l.Directed && l.End1 == t {
				other = l.End2
			}
		case -1:
			if l.Directed && l.End2 == t {
				other = l.End1
			}
		default:
			if l.End1 == t {
				other = l.End2
			} else if l.End2 == t {
				other = l.End1
			}
		}
		if other != nil && !other.dead && !seen[other] {
			seen[other] = true
			out = append(out, other)
		}
	}
	return out
}

// --- diffuse ---

// Diffuse shares fraction of a patch variable with the 8 neighbors.
// Shares that would leave a non-wrapping world are lost.
func (w *World) patchVarNum(p *Patch, varName string) (float64, error) {
	switch varName {
	case "pcolor":
		return p.PColor, nil
	case "pxcor":
		return float64(p.PX), nil
	case "pycor":
		return float64(p.PY), nil
	case "pzcor":
		if w.threeD {
			return float64(p.PZ), nil
		}
	}
	v, ok := p.Vars[varName]
	if !ok {
		return 0, fmt.Errorf("there is no patch variable called %s", varName)
	}
	return v.Num()
}

// Hill moves turtle to the neighboring patch with the highest (uphill) or
// lowest (downhill) value of varName, breaking ties randomly. If no neighbor
// is higher (uphill) or lower (downhill) than the turtle's current patch,
// the turtle does not move.
func (w *World) Hill(t *Turtle, varName string, four, down bool) error {
	curP := w.PatchAt3(t.X, t.Y, t.Z)
	if curP == nil {
		return nil
	}
	curVal, err := w.patchVarNum(curP, varName)
	if err != nil {
		return err
	}
	var nbs []*Patch
	if four {
		nbs = w.Neighbors4(curP)
	} else {
		nbs = w.Neighbors(curP)
	}
	if len(nbs) == 0 {
		return nil
	}
	bestVal := curVal
	var bestPatches []*Patch
	for _, p := range nbs {
		val, err := w.patchVarNum(p, varName)
		if err != nil {
			return err
		}
		if down {
			if val < bestVal {
				bestVal = val
				bestPatches = []*Patch{p}
			} else if val == bestVal && len(bestPatches) > 0 {
				bestPatches = append(bestPatches, p)
			}
		} else {
			if val > bestVal {
				bestVal = val
				bestPatches = []*Patch{p}
			} else if val == bestVal && len(bestPatches) > 0 {
				bestPatches = append(bestPatches, p)
			}
		}
	}
	if len(bestPatches) == 0 {
		return nil
	}
	// NetLogo always draws nextInt(winners), even for a single winner.
	pick := bestPatches[w.nextInt(len(bestPatches))]
	if w.threeD {
		px, py, pz := float64(pick.PX), float64(pick.PY), float64(pick.PZ)
		if err := w.FaceXYZ(t, px, py, pz, true); err != nil {
			return err
		}
		return w.MoveTo3(t, px, py, pz)
	}
	h, err := w.HeadingTowards(t.X, t.Y, float64(pick.PX), float64(pick.PY))
	if err == nil {
		if err := w.SetHeading(t, h); err != nil {
			return err
		}
	}
	return w.MoveTo(t, float64(pick.PX), float64(pick.PY))
}

func (w *World) Diffuse(varName string, amount float64, four bool) error {
	if amount < 0 || amount > 1 {
		return fmt.Errorf("diffuse amount must be between 0 and 1")
	}
	n := len(w.patches)
	next := make([]float64, n)
	cur := make([]float64, n)
	for i, p := range w.patches {
		v, ok := p.Vars[varName]
		if !ok {
			// pcolor is not in Vars; allow diffusing pcolor via the field
			if varName == "pcolor" {
				cur[i] = p.PColor
				continue
			}
			return fmt.Errorf("there is no patch variable called %s", varName)
		}
		num, err := v.Num()
		if err != nil {
			return fmt.Errorf("diffuse expected %s to be a number", varName)
		}
		cur[i] = num
	}
	if w.threeD {
		// NetLogo 3D's diffuse shares with all 26 neighbors. diffuse4
		// shares with the four side neighbors on the same layer.
		offs := neighborOffs26
		if four {
			offs = neighborOffs4
		}
		give := amount / float64(len(offs))
		for i, p := range w.patches {
			next[i] += cur[i] * (1 - amount)
			share := cur[i] * give
			for _, o := range offs {
				np := w.PatchAtPcor3(p.PX+o[0], p.PY+o[1], p.PZ+o[2])
				if np == nil {
					continue
				}
				next[w.patchIndex3(np.PX, np.PY, np.PZ)] += share
			}
		}
	} else {
		shareN := 8
		if four {
			shareN = 4
		}
		offs := [][2]int{{-1, 1}, {0, 1}, {1, 1}, {-1, 0}, {1, 0}, {-1, -1}, {0, -1}, {1, -1}}
		if four {
			offs = [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
		}
		for i, p := range w.patches {
			keep := cur[i] * (1 - amount)
			give := cur[i] * amount / float64(shareN)
			next[i] += keep
			for _, o := range offs {
				np := w.PatchAtPcor(p.PX+o[0], p.PY+o[1])
				if np == nil {
					continue
				}
				j := w.patchIndex(np.PX, np.PY)
				next[j] += give
			}
		}
	}
	for i, p := range w.patches {
		if varName == "pcolor" {
			p.PColor = next[i]
		} else {
			p.Vars[varName] = Num(next[i])
		}
	}
	return nil
}

// --- variable access ---

func (w *World) turtleOwns(t *Turtle, name string) bool {
	for _, n := range w.turtlesOwn {
		if n == name {
			return true
		}
	}
	if t.Breed != "" {
		for _, n := range w.breedOwn[t.Breed] {
			if n == name {
				return true
			}
		}
	}
	return false
}

func (w *World) patchOwns(name string) bool {
	for _, n := range w.patchesOwn {
		if n == name {
			return true
		}
	}
	return false
}

// --- pen drawing and stamps ---

// MaxDrawingMarks is the most pen strokes and stamps a world keeps.
// Robby's fitness loop calls fd once per action. With visuals on, that is
// tens of thousands of strokes per generation, and every published frame
// copies the layer. Older strokes drop off the front. A clear-drawing
// or clear-all still drops the whole layer.
const MaxDrawingMarks = 8192

// drawingLog is a ring of marks in chronological order.
type drawingLog struct {
	buf   []Mark
	start int
	n     int
}

func (d *drawingLog) add(m Mark) {
	if d.n < MaxDrawingMarks {
		d.buf = append(d.buf, m)
		d.n++
		return
	}
	if len(d.buf) != MaxDrawingMarks {
		next := make([]Mark, MaxDrawingMarks)
		copy(next, d.buf)
		d.buf = next
	}
	d.buf[d.start] = m
	d.start = (d.start + 1) % MaxDrawingMarks
}

func (d *drawingLog) at(i int) *Mark {
	idx := i
	if d.n == MaxDrawingMarks {
		idx = (d.start + i) % MaxDrawingMarks
	}
	return &d.buf[idx]
}

func (d *drawingLog) snapshot() []Mark {
	if d == nil || d.n == 0 {
		return nil
	}
	out := make([]Mark, d.n)
	if d.n < MaxDrawingMarks || d.start == 0 {
		copy(out, d.buf[:d.n])
		return out
	}
	n := copy(out, d.buf[d.start:])
	copy(out[n:], d.buf[:d.start])
	return out
}

// Mark is one operation on the drawing layer. Pen is a stroke from
// (X, Y, Z) to (X2, Y2, Z2). Z is 0 in a 2D world. Erase on a pen stroke
// is pen-mode "erase": it removes earlier drawing along the stroke.
// A stamp leaves the turtle's shape at (X, Y, Z). Erase on a stamp is
// stamp-erase.
type Mark struct {
	Pen, Erase    bool
	X, Y, Z       float64
	X2, Y2, Z2    float64
	Heading, Size float64
	Color         float64
	Shape         string
}

// Stamp records the turtle's shape, color, size, and heading on the
// drawing layer. erase removes drawing under that shape.
func (w *World) Stamp(t *Turtle, erase bool) {
	if t == nil || t.dead {
		return
	}
	shape := t.Shape
	if shape == "" {
		shape = "default"
	}
	w.drawing.add(Mark{
		Erase:   erase,
		X:       t.X,
		Y:       t.Y,
		Z:       t.Z,
		X2:      t.X,
		Y2:      t.Y,
		Z2:      t.Z,
		Heading: t.Heading,
		Size:    t.Size,
		Color:   t.Color,
		Shape:   shape,
	})
}

// Marks returns a copy of the drawing layer, oldest first.
func (w *World) Marks() []Mark {
	return w.drawing.snapshot()
}

// ClearDrawing drops pen trails and stamps. Turtles, labels, and patch
// colors stay. clear-all clears the drawing too.
func (w *World) ClearDrawing() {
	w.drawing = drawingLog{}
}

func (w *World) drawLine(x1, y1, z1, x2, y2, z2, col, size float64, erase bool) {
	if size <= 0 {
		size = 1
	}
	if x1 == x2 && y1 == y2 && z1 == z2 {
		return
	}
	w.drawing.add(Mark{
		Pen:   true,
		Erase: erase,
		X:     x1,
		Y:     y1,
		Z:     z1,
		X2:    x2,
		Y2:    y2,
		Z2:    z2,
		Size:  size,
		Color: col,
	})
}

// DrawingAt reports the ink on a patch after replaying the drawing layer.
// The center of the patch is what counts. ok is false when that point is bare.
func (w *World) DrawingAt(px, py int) (colorRGBA, bool) {
	if !w.inBounds(px, py) {
		return colorRGBA{}, false
	}
	var cur colorRGBA
	hit := false
	for i := 0; i < w.drawing.n; i++ {
		m := w.drawing.at(i)
		if m.Pen {
			if !w.segmentHitsPatch(m, px, py) {
				continue
			}
			if m.Erase {
				cur = colorRGBA{}
				hit = false
				continue
			}
			c := ColorRGBA(m.Color)
			cur = colorRGBA{c.R, c.G, c.B, 255}
			hit = true
			continue
		}
		if !w.stampCovers(*m, px, py) {
			continue
		}
		if m.Erase {
			cur = colorRGBA{}
			hit = false
			continue
		}
		c := ColorRGBA(m.Color)
		cur = colorRGBA{c.R, c.G, c.B, 255}
		hit = true
	}
	return cur, hit
}

func (w *World) segmentHitsPatch(m *Mark, px, py int) bool {
	steps := int(math.Hypot(m.X2-m.X, m.Y2-m.Y)*2) + 1
	if steps < 1 {
		steps = 1
	}
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		x := m.X + (m.X2-m.X)*f
		y := m.Y + (m.Y2-m.Y)*f
		p := w.PatchAt(x, y)
		if p != nil && p.PX == px && p.PY == py {
			return true
		}
	}
	return false
}

// Shuffle returns agents in the order NetLogo's shufflerator yields them,
// drawing every index up front. Dead agents keep their slot in the draw
// sequence and stay in the result. Primitives that run code per agent
// use ShuffleIter so draws interleave with the body as in NetLogo.
func (w *World) Shuffle(ags []Agent) []Agent {
	it := w.newShuffler(ags, false)
	out := make([]Agent, 0, len(ags))
	for {
		a, ok := it.Next()
		if !ok {
			return out
		}
		out = append(out, a)
	}
}

// SetBreed moves a turtle into a breed. Empty breed means the default turtles.
func (w *World) SetBreed(t *Turtle, plural string) error {
	if plural != "" {
		if _, ok := w.breeds[plural]; !ok {
			return fmt.Errorf("there is no breed called %s", plural)
		}
	}
	w.untrackBreed(t)
	t.Breed = plural
	w.trackBreed(t)
	if t.Vars == nil {
		t.Vars = map[string]Value{}
	}
	for _, name := range w.breedOwn[plural] {
		if _, ok := t.Vars[name]; !ok {
			t.Vars[name] = Num(0)
		}
	}
	if _, ok := w.defaultShape[plural]; ok && (t.Shape == "default" || t.Shape == w.defaultShape["turtles"]) {
		t.Shape = w.defaultShape[plural]
	}
	return nil
}
