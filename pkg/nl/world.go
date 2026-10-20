package nl

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// World is the NetLogo world: a patch grid, turtles, links, and the observer.
type World struct {
	MinPxcor, MaxPxcor int
	MinPycor, MaxPycor int
	WrapX, WrapY       bool
	PatchSize          float64

	width, height int
	patches       []*Patch
	// patchOrder is NetLogo's patch iteration order: top to bottom, left to right.
	patchOrder []*Patch

	turtles map[int]*Turtle
	whoSeq  []int // ascending who, including dead slots compacted lazily
	nextWho int

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

	defaultShape map[string]string // breed plural or "turtles" -> shape

	ticks        float64
	ticksStarted bool
	// runEpoch increments on clear-ticks, reset-ticks, and clear-all.
	// The run timer watches it. Tick does not, so a benchmark is unchanged.
	runEpoch uint64

	rng  *rand.Rand
	seed int64

	// marks is the drawing layer: pen strokes and stamps, oldest first.
	// clear-drawing and clear-all drop it. Nil until something is drawn.
	// Turtle, Patch, and Link labels keep their value type. The view
	// formats them with DisplayLabel when it paints. label-color and
	// plabel-color stay in Vars.
	marks []Mark

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
	return fmt.Sprintf("(patch %d %d)", p.PX, p.PY)
}

// Turtle is a mobile agent.
type Turtle struct {
	Who     int
	X, Y    float64
	Heading float64
	Color   float64
	Shape   string
	Size    float64
	Hidden  bool
	Label   Value
	Breed   string // plural; empty means the default turtles breed
	PenDown bool
	PenSize float64
	Vars    map[string]Value
	dead    bool
	world   *World
	patch   *Patch
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
		turtles:      map[int]*Turtle{},
		globals:      map[string]Value{},
		ifaceGlob:    map[string]bool{},
		breeds:       map[string]*breedDef{},
		linkBreeds:   map[string]*linkBreedDef{},
		breedOwn:     map[string][]string{},
		defaultShape: map[string]string{"turtles": "default"},
	}
	w.recreatePatches()
	w.Seed(time.Now().UnixNano())
	return w
}

func (w *World) recreatePatches() {
	w.width = w.MaxPxcor - w.MinPxcor + 1
	w.height = w.MaxPycor - w.MinPycor + 1
	if w.width < 1 || w.height < 1 {
		panic("world size must be positive")
	}
	w.patches = make([]*Patch, w.width*w.height)
	w.patchOrder = make([]*Patch, 0, w.width*w.height)
	for y := w.MaxPycor; y >= w.MinPycor; y-- {
		for x := w.MinPxcor; x <= w.MaxPxcor; x++ {
			p := &Patch{
				PX: x, PY: y,
				PColor: 0,
				Vars:   map[string]Value{},
				Label:  Str(""),
				world:  w,
			}
			for _, name := range w.patchesOwn {
				p.Vars[name] = Num(0)
			}
			w.patches[w.patchIndex(x, y)] = p
			w.patchOrder = append(w.patchOrder, p)
		}
	}
	w.marks = nil
}

func (w *World) patchIndex(px, py int) int {
	return (py-w.MinPycor)*w.width + (px - w.MinPxcor)
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

// ApplyView resizes the world and sets patch size and wrapping.
// Turtles and links die and patches are reset, as with resize-world.
// Patch size must be positive and at most 100.
func (w *World) ApplyView(minPx, maxPx, minPy, maxPy int, patchSize float64, wrapX, wrapY bool) error {
	if err := ValidWorld(minPx, maxPx, minPy, maxPy); err != nil {
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
// is left alone, matching clear-turtles rather than clear-all.
func (w *World) Resize(minPx, maxPx, minPy, maxPy int, wrapX, wrapY bool) {
	w.MinPxcor, w.MaxPxcor = minPx, maxPx
	w.MinPycor, w.MaxPycor = minPy, maxPy
	w.WrapX, w.WrapY = wrapX, wrapY
	w.turtles = map[int]*Turtle{}
	w.whoSeq = nil
	w.links = nil
	w.recreatePatches()
}

func (w *World) Seed(seed int64) {
	w.seed = seed
	w.rng = rand.New(rand.NewSource(seed))
}

func (w *World) RNG() *rand.Rand { return w.rng }

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
func (w *World) wrapCoordX(x float64) (float64, error) {
	min := float64(w.MinPxcor) - 0.5
	max := float64(w.MaxPxcor) + 0.5
	if w.WrapX {
		return wrapDouble(x, min, max), nil
	}
	if x >= max || x < min {
		return x, fmt.Errorf("the turtle cannot move beyond the world's edge")
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
		return y, fmt.Errorf("the turtle cannot move beyond the world's edge")
	}
	return y, nil
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
	w.ClearGlobals()
	w.ClearTicks()
	w.ClearTurtles()
	w.ClearLinks()
	w.ClearPatches()
	w.ClearDrawing()
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

func (w *World) ClearTurtles() {
	for _, t := range w.turtles {
		t.dead = true
		t.patch = nil
	}
	w.turtles = map[int]*Turtle{}
	w.whoSeq = nil
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

func (w *World) TurtlesOfBreed(plural string) *AgentSet {
	live := w.liveTurtles()
	ags := make([]Agent, 0, len(live))
	for _, t := range live {
		if t.Breed == plural {
			ags = append(ags, t)
		}
	}
	return &AgentSet{Kind: KindTurtle, Breed: plural, Agents: ags}
}

func (w *World) Turtle(who int) *Turtle {
	t := w.turtles[who]
	if t == nil || t.dead {
		return nil
	}
	return t
}

func (w *World) Patches() *AgentSet {
	ags := make([]Agent, len(w.patchOrder))
	for i, p := range w.patchOrder {
		ags[i] = p
	}
	return &AgentSet{Kind: KindPatch, All: true, Agents: ags}
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
	p := w.PatchAt(t.X, t.Y)
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
			p.turtles = ts[:len(ts)-1]
			break
		}
	}
	t.patch = nil
}

// CreateTurtle creates one turtle. colorN in 0..13 selects a base hue
// (5+10*colorN). heading is in degrees. Position is (x, y).
func (w *World) CreateTurtle(breed string, x, y float64, colorN, heading int) *Turtle {
	t := &Turtle{
		Who:     w.nextWho,
		X:       x,
		Y:       y,
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
		Who:     w.nextWho,
		X:       parent.X,
		Y:       parent.Y,
		Heading: parent.Heading,
		Color:   parent.Color,
		Shape:   parent.Shape,
		Size:    parent.Size,
		Hidden:  parent.Hidden,
		Label:   parent.Label,
		Breed:   b,
		PenDown: parent.PenDown,
		PenSize: parent.PenSize,
		world:   w,
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
}

// MoveTo sets a turtle's position, wrapping or rejecting at the edge.
func (w *World) MoveTo(t *Turtle, x, y float64) error {
	nx, err := w.wrapCoordX(x)
	if err != nil {
		return err
	}
	ny, err := w.wrapCoordY(y)
	if err != nil {
		return err
	}
	if t.PenDown {
		w.drawLine(t.X, t.Y, nx, ny, t.Color, t.PenSize)
	}
	w.detach(t)
	t.X, t.Y = nx, ny
	w.attach(t)
	return nil
}

func (w *World) Forward(t *Turtle, dist float64) error {
	rad := t.Heading * math.Pi / 180
	return w.MoveTo(t, t.X+dist*math.Sin(rad), t.Y+dist*math.Cos(rad))
}

func (w *World) CanMove(t *Turtle, dist float64) bool {
	rad := t.Heading * math.Pi / 180
	x := t.X + dist*math.Sin(rad)
	y := t.Y + dist*math.Cos(rad)
	if _, err := w.wrapCoordX(x); err != nil {
		return false
	}
	if _, err := w.wrapCoordY(y); err != nil {
		return false
	}
	return true
}

func (w *World) SetHeading(t *Turtle, h float64) {
	t.Heading = h
	w.normalizeHeading(t)
}

func (w *World) Turn(t *Turtle, delta float64) {
	w.SetHeading(t, t.Heading+delta)
}

// PatchHere is the patch under a turtle.
func (w *World) PatchHere(t *Turtle) *Patch {
	if t.patch != nil && !t.patch.dead {
		return t.patch
	}
	return w.PatchAt(t.X, t.Y)
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
	// who order
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j].Who < out[j-1].Who {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out
}

// Neighbors returns the 8 surrounding patches (fewer at the edge of a box).
func (w *World) Neighbors(p *Patch) []*Patch {
	return w.neighbors(p, false)
}

func (w *World) Neighbors4(p *Patch) []*Patch {
	return w.neighbors(p, true)
}

func (w *World) neighbors(p *Patch, four bool) []*Patch {
	var offs [][2]int
	if four {
		offs = [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
	} else {
		offs = [][2]int{
			{-1, 1}, {0, 1}, {1, 1},
			{-1, 0}, {1, 0},
			{-1, -1}, {0, -1}, {1, -1},
		}
	}
	out := make([]*Patch, 0, len(offs))
	for _, o := range offs {
		np := w.PatchAtPcor(p.PX+o[0], p.PY+o[1])
		if np != nil {
			out = append(out, np)
		}
	}
	return out
}

// ShortestDelta is the signed offset from a to b, wrapping when asked.
func (w *World) ShortestDX(x1, x2 float64) float64 {
	d := x2 - x1
	if !w.WrapX {
		return d
	}
	size := w.worldWidth()
	if d > size/2 {
		d -= size
	} else if d <= -size/2 {
		d += size
	}
	return d
}

func (w *World) ShortestDY(y1, y2 float64) float64 {
	d := y2 - y1
	if !w.WrapY {
		return d
	}
	size := w.worldHeight()
	if d > size/2 {
		d -= size
	} else if d <= -size/2 {
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
// 0 is north, 90 is east.
func (w *World) HeadingTowards(x1, y1, x2, y2 float64) float64 {
	dx := w.ShortestDX(x1, x2)
	dy := w.ShortestDY(y1, y2)
	if dx == 0 && dy == 0 {
		return 0
	}
	h := math.Atan2(dx, dy) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}

func (w *World) RandomXcor() float64 {
	span := w.worldWidth()
	return float64(w.MinPxcor) - 0.5 + w.rng.Float64()*span
}

func (w *World) RandomYcor() float64 {
	span := w.worldHeight()
	return float64(w.MinPycor) - 0.5 + w.rng.Float64()*span
}

func (w *World) RandomPxcor() int {
	return w.MinPxcor + w.rng.Intn(w.width)
}

func (w *World) RandomPycor() int {
	return w.MinPycor + w.rng.Intn(w.height)
}

// RandomInt implements random. n > 0 yields [0, n). n < 0 yields (n, 0].
func (w *World) RandomInt(n int) (int, error) {
	if n == 0 {
		return 0, fmt.Errorf("random expected a nonzero number")
	}
	if n > 0 {
		return w.rng.Intn(n), nil
	}
	return -w.rng.Intn(-n), nil
}

func (w *World) RandomFloat(n float64) (float64, error) {
	if n == 0 {
		return 0, fmt.Errorf("random-float expected a nonzero number")
	}
	if n > 0 {
		return w.rng.Float64() * n, nil
	}
	return -w.rng.Float64() * -n, nil
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
	return l, true, nil
}

func (b *linkBreedDef) ownOrEmpty() []string {
	if b == nil {
		return nil
	}
	return b.Own
}

func (w *World) LinksOf(t *Turtle) []*Link {
	var out []*Link
	for _, l := range w.links {
		if l.dead {
			continue
		}
		if l.End1 == t || l.End2 == t {
			out = append(out, l)
		}
	}
	return out
}

func (w *World) LinkNeighbors(t *Turtle, dir int) []*Turtle {
	// dir: 0 either, 1 out, -1 in
	seen := map[*Turtle]bool{}
	var out []*Turtle
	for _, l := range w.links {
		if l.dead {
			continue
		}
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

// Mark is one operation on the drawing layer. Pen is a stroke from
// (X, Y) to (X2, Y2). A stamp leaves the turtle's shape at (X, Y).
type Mark struct {
	Pen, Erase    bool
	X, Y, X2, Y2  float64
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
	w.marks = append(w.marks, Mark{
		Erase:   erase,
		X:       t.X,
		Y:       t.Y,
		X2:      t.X,
		Y2:      t.Y,
		Heading: t.Heading,
		Size:    t.Size,
		Color:   t.Color,
		Shape:   shape,
	})
}

// Marks returns a copy of the drawing layer, oldest first.
func (w *World) Marks() []Mark {
	if len(w.marks) == 0 {
		return nil
	}
	out := make([]Mark, len(w.marks))
	copy(out, w.marks)
	return out
}

// ClearDrawing drops pen trails and stamps. Turtles, labels, and patch
// colors stay. clear-all clears the drawing too.
func (w *World) ClearDrawing() {
	w.marks = nil
}

func (w *World) drawLine(x1, y1, x2, y2, col, size float64) {
	if size <= 0 {
		size = 1
	}
	w.marks = append(w.marks, Mark{
		Pen:   true,
		X:     x1,
		Y:     y1,
		X2:    x2,
		Y2:    y2,
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
	for i := range w.marks {
		m := &w.marks[i]
		if m.Pen {
			if !w.segmentHitsPatch(m, px, py) {
				continue
			}
			c := ColorRGBA(m.Color)
			cur = colorRGBA{c.R, c.G, c.B, 255}
			hit = true
			continue
		}
		if !shapeCovers(*m, px, py) {
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

// Shuffle copies agents and shuffles them with the world RNG.
func (w *World) Shuffle(ags []Agent) []Agent {
	out := append([]Agent(nil), ags...)
	w.rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// SetBreed moves a turtle into a breed. Empty breed means the default turtles.
func (w *World) SetBreed(t *Turtle, plural string) error {
	if plural != "" {
		if _, ok := w.breeds[plural]; !ok {
			return fmt.Errorf("there is no breed called %s", plural)
		}
	}
	t.Breed = plural
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
