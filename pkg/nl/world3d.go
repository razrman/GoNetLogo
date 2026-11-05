package nl

import (
	"fmt"
	"image/color"
	"math"
)

// This file holds the NetLogo 3D geometry: the Z axis, wrapping in Z,
// 3D distances and pitch, and the heading/pitch/roll orientation frame.
// A 2D world never takes these paths; every helper either checks
// w.threeD or is only called from a 3D primitive.

// MaxPatches3D caps the number of patches in a 3D world. Each axis is
// still limited by MaxWorldSide. The cap keeps a large cube from
// allocating hundreds of megabytes of patches.
const MaxPatches3D = 1 << 20

// NewWorld3D builds a NetLogo 3D world. NetLogo 3D worlds are tori, so
// every axis wraps no matter what the model file says.
func NewWorld3D(minPx, maxPx, minPy, maxPy, minPz, maxPz int) *World {
	w := NewWorld(minPx, maxPx, minPy, maxPy, true, true)
	w.threeD = true
	w.WrapZ = true
	w.MinPzcor, w.MaxPzcor = minPz, maxPz
	w.recreatePatches()
	w.homeObserver()
	return w
}

// Is3D reports whether this is a NetLogo 3D world.
func (w *World) Is3D() bool { return w != nil && w.threeD }

// Depth is the number of patch layers. It is 1 in a 2D world.
func (w *World) Depth() int { return w.depth }

func (w *World) worldDepth() float64 { return float64(w.depth) }

// ValidWorld3D is ValidWorld with a Z axis. The world must contain the
// origin, stay within MaxWorldSide on each axis, and hold at most
// MaxPatches3D patches.
func ValidWorld3D(minPx, maxPx, minPy, maxPy, minPz, maxPz int) error {
	if err := ValidWorld(minPx, maxPx, minPy, maxPy); err != nil {
		return err
	}
	if minPz > 0 || maxPz < 0 {
		return fmt.Errorf("the world must contain the origin")
	}
	if maxPz-minPz+1 > MaxWorldSide {
		return fmt.Errorf("a world can be at most %d patches on a side", MaxWorldSide)
	}
	n := (maxPx - minPx + 1) * (maxPy - minPy + 1) * (maxPz - minPz + 1)
	if n > MaxPatches3D {
		return fmt.Errorf("a 3D world can have at most %d patches, this one has %d", MaxPatches3D, n)
	}
	return nil
}

// Resize3 is resize-world in NetLogo 3D: six bounds, every axis wraps.
// Turtles and links die and patches are rebuilt, as with Resize.
func (w *World) Resize3(minPx, maxPx, minPy, maxPy, minPz, maxPz int) {
	w.MinPzcor, w.MaxPzcor = minPz, maxPz
	w.Resize(minPx, maxPx, minPy, maxPy, true, true)
	w.homeObserver()
}

func (w *World) wrapCoordZ(z float64) (float64, error) {
	if !w.threeD {
		return z, nil
	}
	min := float64(w.MinPzcor) - 0.5
	max := float64(w.MaxPzcor) + 0.5
	if w.WrapZ {
		return wrapDouble(z, min, max), nil
	}
	if z >= max || z < min {
		return z, errOutsideWorld
	}
	return z, nil
}

func outsidePoint3(x, y, z float64) error {
	return &edgeError{msg: fmt.Sprintf("the point [%s %s %s] is outside of the world", formatNum(x), formatNum(y), formatNum(z))}
}

func (w *World) inBounds3(px, py, pz int) bool {
	return w.inBounds(px, py) && pz >= w.MinPzcor && pz <= w.MaxPzcor
}

// PatchAt3 is the patch containing (x, y, z). A 2D world ignores z.
func (w *World) PatchAt3(x, y, z float64) *Patch {
	if !w.threeD {
		return w.PatchAt(x, y)
	}
	if w.WrapX {
		x = wrapDouble(x, float64(w.MinPxcor)-0.5, float64(w.MaxPxcor)+0.5)
	}
	if w.WrapY {
		y = wrapDouble(y, float64(w.MinPycor)-0.5, float64(w.MaxPycor)+0.5)
	}
	if w.WrapZ {
		z = wrapDouble(z, float64(w.MinPzcor)-0.5, float64(w.MaxPzcor)+0.5)
	}
	px, py, pz := w.roundPatch(x), w.roundPatch(y), w.roundPatch(z)
	if !w.inBounds3(px, py, pz) {
		return nil
	}
	return w.patches[w.patchIndex3(px, py, pz)]
}

// PatchAtPcor3 is the patch at integer coordinates, wrapping when the
// axis wraps. A 2D world ignores pz.
func (w *World) PatchAtPcor3(px, py, pz int) *Patch {
	if !w.threeD {
		return w.PatchAtPcor(px, py)
	}
	if w.WrapX {
		px = wrapPcorJava(px, w.MinPxcor, w.MaxPxcor)
	}
	if w.WrapY {
		py = wrapPcorJava(py, w.MinPycor, w.MaxPycor)
	}
	if w.WrapZ {
		pz = wrapPcorJava(pz, w.MinPzcor, w.MaxPzcor)
	}
	if !w.inBounds3(px, py, pz) {
		return nil
	}
	return w.patches[w.patchIndex3(px, py, pz)]
}

// GetPatch3 is the patch at exact integer coordinates, or nil.
func (w *World) GetPatch3(px, py, pz int) *Patch {
	if !w.inBounds3(px, py, pz) {
		return nil
	}
	return w.patches[w.patchIndex3(px, py, pz)]
}

// ShortestDZ is ShortestDX for the Z axis. A 2D world reports 0.
func (w *World) ShortestDZ(z1, z2 float64) float64 {
	if !w.threeD {
		return 0
	}
	d := z2 - z1
	if !w.WrapZ {
		return d
	}
	return shortestWrap(d, w.worldDepth())
}

// Distance3 is the wrapped distance between two points. In a 2D world it
// is exactly Distance, so 2D results do not change.
func (w *World) Distance3(x1, y1, z1, x2, y2, z2 float64) float64 {
	if !w.threeD {
		return w.Distance(x1, y1, x2, y2)
	}
	dx := w.ShortestDX(x1, x2)
	dy := w.ShortestDY(y1, y2)
	dz := w.ShortestDZ(z1, z2)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// distanceNoWrap is straight-line distance, ignoring wrapping.
func distanceNoWrap(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx, dy, dz := x2-x1, y2-y1, z2-z1
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// delta3 is the offset from point 1 to point 2, the short way around a
// wrapping axis when wrap is true.
func (w *World) delta3(x1, y1, z1, x2, y2, z2 float64, wrap bool) (float64, float64, float64) {
	if !wrap {
		return x2 - x1, y2 - y1, z2 - z1
	}
	return w.ShortestDX(x1, x2), w.ShortestDY(y1, y2), w.ShortestDZ(z1, z2)
}

// pitchOf is the NetLogo pitch of an offset: the elevation angle above
// the xy plane, in [0, 360). ok is false for a zero offset.
func pitchOf(dx, dy, dz float64) (float64, bool) {
	if dx == 0 && dy == 0 && dz == 0 {
		return 0, false
	}
	return normDeg(math.Atan2(dz, math.Hypot(dx, dy)) * 180 / math.Pi), true
}

// headingOf is the NetLogo heading of an xy offset. ok is false when the
// offset is vertical or zero.
func headingOf(dx, dy float64) (float64, bool) {
	if dx == 0 && dy == 0 {
		return 0, false
	}
	return normDeg(math.Atan2(dx, dy) * 180 / math.Pi), true
}

// PitchTowards is towards-pitch: the pitch from point 1 to point 2.
func (w *World) PitchTowards(x1, y1, z1, x2, y2, z2 float64, wrap bool) (float64, error) {
	dx, dy, dz := w.delta3(x1, y1, z1, x2, y2, z2, wrap)
	p, ok := pitchOf(dx, dy, dz)
	if !ok {
		return 0, fmt.Errorf("no pitch is defined from a point (%s,%s,%s) to that same point", formatNum(x1), formatNum(y1), formatNum(z1))
	}
	return p, nil
}

// HeadingTowards3 is towards in 3D: the heading of the xy part of the
// offset. wrap false is the -nowrap form.
func (w *World) HeadingTowards3(x1, y1, z1, x2, y2, z2 float64, wrap bool) (float64, error) {
	dx, dy, _ := w.delta3(x1, y1, z1, x2, y2, z2, wrap)
	h, ok := headingOf(dx, dy)
	if !ok {
		return 0, fmt.Errorf("no heading is defined from a point (%s,%s,%s) to a point directly above or below it", formatNum(x1), formatNum(y1), formatNum(z1))
	}
	return h, nil
}

// FaceXYZ turns t toward the point: heading from the xy offset and pitch
// from the elevation. When the point is straight above or below, the
// heading is kept. When it is the turtle's own position nothing changes.
// Roll is left alone.
func (w *World) FaceXYZ(t *Turtle, x, y, z float64, wrap bool) error {
	dx, dy, dz := w.delta3(t.X, t.Y, t.Z, x, y, z, wrap)
	p, ok := pitchOf(dx, dy, dz)
	if !ok {
		return nil
	}
	if h, ok := headingOf(dx, dy); ok {
		if err := w.SetHeading(t, h); err != nil {
			return err
		}
	}
	t.Pitch = p
	return nil
}

func normDeg(d float64) float64 {
	d = math.Mod(d, 360)
	if d < 0 {
		d += 360
	}
	if d >= 360 {
		d = 0
	}
	// Turn -0 into 0 so it prints as 0.
	return d + 0
}

// orientDelta is the displacement for moving dist along heading and pitch:
// dx = cos(pitch) sin(heading), dy = cos(pitch) cos(heading), dz = sin(pitch).
func orientDelta(heading, pitch, dist float64) (float64, float64, float64) {
	sh, ch := math.Sincos(heading * math.Pi / 180)
	sp, cp := math.Sincos(pitch * math.Pi / 180)
	return dist * cp * sh, dist * cp * ch, dist * sp
}

// --- orientation frame ---

type vec3 [3]float64

func (a vec3) dot(b vec3) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func (a vec3) cross(b vec3) vec3 {
	return vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func (a vec3) scale(k float64) vec3 { return vec3{a[0] * k, a[1] * k, a[2] * k} }

func (a vec3) add(b vec3) vec3 { return vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }

func (a vec3) unit() vec3 {
	n := math.Sqrt(a.dot(a))
	if n == 0 {
		return a
	}
	return a.scale(1 / n)
}

// rotateAbout turns v by deg degrees around the unit axis k, counter-
// clockwise when looking down k toward the origin (Rodrigues' formula).
func rotateAbout(v, k vec3, deg float64) vec3 {
	s, c := math.Sincos(deg * math.Pi / 180)
	return v.scale(c).add(k.cross(v).scale(s)).add(k.scale(k.dot(v) * (1 - c)))
}

// unrolledFrame is the forward, up, and right vectors for heading and
// pitch with roll 0. Up is the forward vector tilted 90 degrees upward;
// right is level.
func unrolledFrame(heading, pitch float64) (f, u, r vec3) {
	sh, ch := math.Sincos(heading * math.Pi / 180)
	sp, cp := math.Sincos(pitch * math.Pi / 180)
	f = vec3{cp * sh, cp * ch, sp}
	u = vec3{-sp * sh, -sp * ch, cp}
	r = vec3{ch, -sh, 0}
	return f, u, r
}

// orientFrame is the turtle's forward, up, and right vectors. Positive
// roll tips the up vector toward the right, which is roll-right.
func orientFrame(heading, pitch, roll float64) (f, u, r vec3) {
	f, u0, r0 := unrolledFrame(heading, pitch)
	sr, cr := math.Sincos(roll * math.Pi / 180)
	u = u0.scale(cr).add(r0.scale(sr))
	r = r0.scale(cr).add(u0.scale(-sr))
	return f, u, r
}

// frameAngles converts forward and up vectors back to heading, pitch, and
// roll. When forward is vertical the heading is undefined; prevHeading is
// kept and the roll absorbs the rest of the rotation.
func frameAngles(f, u vec3, prevHeading float64) (heading, pitch, roll float64) {
	f = f.unit()
	u = u.unit()
	fz := math.Max(-1, math.Min(1, f[2]))
	pitch = math.Asin(fz) * 180 / math.Pi
	if math.Hypot(f[0], f[1]) < 1e-12 {
		heading = prevHeading
	} else {
		heading = math.Atan2(f[0], f[1]) * 180 / math.Pi
	}
	heading = normDeg(heading)
	pitch = normDeg(pitch)
	_, u0, r0 := unrolledFrame(heading, pitch)
	roll = normDeg(math.Atan2(u.dot(r0), u.dot(u0)) * 180 / math.Pi)
	return heading, pitch, roll
}

func (t *Turtle) setOrientation(h, p, r float64) {
	t.Heading, t.Pitch, t.Roll = normDeg(h), normDeg(p), normDeg(r)
}

// TurnRight3D is right in NetLogo 3D: a turn about the turtle's own up
// vector, which can change pitch and roll as well as heading. A level,
// unrolled turtle only changes heading, through the same Turn path that
// 2D uses, so tied turtles follow.
func (w *World) TurnRight3D(t *Turtle, delta float64) error {
	if t.Pitch == 0 && t.Roll == 0 {
		return w.Turn(t, delta)
	}
	f, u, _ := orientFrame(t.Heading, t.Pitch, t.Roll)
	f = rotateAbout(f, u, -delta)
	h, p, r := frameAngles(f, u, t.Heading)
	t.setOrientation(h, p, r)
	return nil
}

// TiltUp is tilt-up: a turn about the turtle's right vector that raises
// the nose. With no roll that is pitch + delta.
func (w *World) TiltUp(t *Turtle, delta float64) {
	if t.Roll == 0 {
		t.Pitch = normDeg(t.Pitch + delta)
		return
	}
	f, u, r := orientFrame(t.Heading, t.Pitch, t.Roll)
	f = rotateAbout(f, r, delta)
	u = rotateAbout(u, r, delta)
	h, p, ro := frameAngles(f, u, t.Heading)
	t.setOrientation(h, p, ro)
}

// RollRight is roll-right: a turn about the forward vector. Roll is
// measured about that same vector, so it is roll + delta.
func (w *World) RollRight(t *Turtle, delta float64) {
	t.Roll = normDeg(t.Roll + delta)
}

// --- neighbors ---

var neighborOffs26 = func() [][3]int {
	// The 2D order on the patch's own layer, then the layer above, then
	// the layer below.
	ring := [][2]int{{-1, 1}, {0, 1}, {1, 1}, {-1, 0}, {1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	var out [][3]int
	for _, o := range ring {
		out = append(out, [3]int{o[0], o[1], 0})
	}
	for _, dz := range []int{1, -1} {
		out = append(out, [3]int{0, 0, dz})
		for _, o := range ring {
			out = append(out, [3]int{o[0], o[1], dz})
		}
	}
	return out
}()

var neighborOffs6 = [][3]int{{0, 1, 0}, {1, 0, 0}, {0, -1, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}}

var neighborOffs4 = [][3]int{{0, 1, 0}, {1, 0, 0}, {0, -1, 0}, {-1, 0, 0}}

// neighborsAt lists the distinct patches at the offsets. A wrapping axis
// only one or two patches long folds offsets onto the same patch, or back
// onto p; those are dropped, as in 2D.
func (w *World) neighborsAt(p *Patch, offs [][3]int) []*Patch {
	out := make([]*Patch, 0, len(offs))
	for _, o := range offs {
		np := w.PatchAtPcor3(p.PX+o[0], p.PY+o[1], p.PZ+o[2])
		if np == nil || np == p || patchListed(out, np) {
			continue
		}
		out = append(out, np)
	}
	return out
}

// Neighbors6 is the six face neighbors of a patch in a 3D world.
func (w *World) Neighbors6(p *Patch) []*Patch {
	return w.neighborsAt(p, neighborOffs6)
}

// --- random coordinates ---

func (w *World) RandomZcor() float64 {
	return float64(w.MinPzcor) - 0.5 + w.rng.Float64()*w.worldDepth()
}

func (w *World) RandomPzcor() int {
	return w.MinPzcor + w.rng.Intn(w.depth)
}

// --- observer camera ---

// The 3D observer is a camera that looks at a target point. Its position
// is kept as spherical coordinates around the target: distance, azimuth
// (degrees counterclockwise from +x, as seen from above), and elevation
// (degrees above the xy plane). Keeping the azimuth separately means a
// camera straight above the target still knows which way to come down
// when orbited.
//
// setxyz moves the camera and keeps the target. facexyz and face move the
// target and keep the camera. orbit-* swing the camera around the target,
// zoom moves it toward the target, and reset-perspective sends it home.
// Every one of those bumps obsRev, which tells a viewer the model moved
// the camera on purpose.

// Observer3D is the observer camera pose of a 3D world.
type Observer3D struct {
	X, Y, Z    float64 // camera position
	TX, TY, TZ float64 // the point it faces
	// Revision counts observer camera commands. It does not change when
	// the model only runs, so a viewer can follow camera commands without
	// fighting the user's own orbiting.
	Revision uint64
}

// homeObserver puts the 3D observer camera where NetLogo 3D's
// reset-perspective does: straight above the center of the world, far
// enough out to see all of it (ozcor is 1.5 times the longest side).
func (w *World) homeObserver() {
	w.obsTX = float64(w.MinPxcor) + float64(w.MaxPxcor-w.MinPxcor)/2
	w.obsTY = float64(w.MinPycor) + float64(w.MaxPycor-w.MinPycor)/2
	w.obsTZ = float64(w.MinPzcor) + float64(w.MaxPzcor-w.MinPzcor)/2
	side := math.Max(float64(w.width), math.Max(float64(w.height), float64(w.depth)))
	w.obsDist = side*1.5 - w.obsTZ
	w.obsAz = 270 // orbiting down from the top ends up south of the world
	w.obsElev = 90
	w.placeObserver()
}

// placeObserver recomputes the camera position from the target and the
// spherical coordinates.
func (w *World) placeObserver() {
	sa, ca := sincosDeg(w.obsAz)
	se, ce := sincosDeg(w.obsElev)
	w.obsX = w.obsTX + w.obsDist*ce*ca
	w.obsY = w.obsTY + w.obsDist*ce*sa
	w.obsZ = w.obsTZ + w.obsDist*se
}

// sincosDeg is sin and cos of an angle in degrees, exact at multiples of
// 90 so a camera straight above the target reports the target's x and y.
func sincosDeg(d float64) (float64, float64) {
	switch normDeg(d) {
	case 0:
		return 0, 1
	case 90:
		return 1, 0
	case 180:
		return 0, -1
	case 270:
		return -1, 0
	}
	return math.Sincos(d * math.Pi / 180)
}

// aimObserver recomputes the spherical coordinates from the camera
// position and target. A camera on the target keeps its old angles.
func (w *World) aimObserver() {
	dx, dy, dz := w.obsX-w.obsTX, w.obsY-w.obsTY, w.obsZ-w.obsTZ
	d := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if d < 1e-9 {
		w.obsDist = 0
		return
	}
	w.obsDist = d
	w.obsElev = math.Asin(math.Max(-1, math.Min(1, dz/d))) * 180 / math.Pi
	if math.Hypot(dx, dy) > 1e-9 {
		w.obsAz = normDeg(math.Atan2(dy, dx) * 180 / math.Pi)
	}
}

// ObserverSetXYZ is observer setxyz: move the camera, keep facing the
// same point.
func (w *World) ObserverSetXYZ(x, y, z float64) {
	w.obsX, w.obsY, w.obsZ = x, y, z
	w.aimObserver()
	w.obsRev++
}

// ObserverFaceXYZ is observer facexyz and face: keep the camera where it
// is and look at the point.
func (w *World) ObserverFaceXYZ(x, y, z float64) {
	w.obsTX, w.obsTY, w.obsTZ = x, y, z
	w.aimObserver()
	w.obsRev++
}

// ObserverOrbit swings the camera around its target. right turns it
// counterclockwise seen from above (the camera moves to its right); up
// raises it. Elevation stops at straight above or straight below, as
// NetLogo's orbit does not flip the camera over.
func (w *World) ObserverOrbit(right, up float64) {
	w.obsAz = normDeg(w.obsAz + right)
	w.obsElev = math.Max(-90, math.Min(90, w.obsElev+up))
	w.placeObserver()
	w.obsRev++
}

// ObserverZoom moves the camera toward its target by d (away when d is
// negative). It never passes the target.
func (w *World) ObserverZoom(d float64) {
	w.obsDist = math.Max(0.01, w.obsDist-d)
	w.placeObserver()
	w.obsRev++
}

// ObserverReset is reset-perspective's camera part.
func (w *World) ObserverReset() {
	w.homeObserver()
	w.obsRev++
}

// ObserverXYZ is the observer camera position in a 3D world.
func (w *World) ObserverXYZ() (float64, float64, float64) { return w.obsX, w.obsY, w.obsZ }

// Observer is the observer camera pose of a 3D world.
func (w *World) Observer() Observer3D {
	return Observer3D{
		X: w.obsX, Y: w.obsY, Z: w.obsZ,
		TX: w.obsTX, TY: w.obsTY, TZ: w.obsTZ,
		Revision: w.obsRev,
	}
}

// Voxel is one non-black patch of a 3D world, for a 3D viewer.
type Voxel struct {
	X, Y, Z int
	C       color.RGBA
}

// Voxels lists the patches of a 3D world whose color is not black, in
// patch order. A 2D world reports nil; its patches are sent as an image.
func (w *World) Voxels() []Voxel {
	if !w.threeD {
		return nil
	}
	var out []Voxel
	for _, p := range w.patchOrder {
		c := patchRGBA(p)
		if c.R == 0 && c.G == 0 && c.B == 0 {
			continue
		}
		out = append(out, Voxel{X: p.PX, Y: p.PY, Z: p.PZ, C: c})
	}
	return out
}

// --- projection for flat views ---

// TopColor is the color a top-down view of a 3D world shows for column
// (px, py): the highest patch on that column that is not black. ok is
// false when every patch on the column is black. In 2D it is the patch
// color.
func (w *World) TopColor(px, py int) (c color.RGBA, ok bool) {
	for pz := w.MaxPzcor; pz >= w.MinPzcor; pz-- {
		p := w.GetPatch3(px, py, pz)
		if p == nil {
			continue
		}
		c := patchRGBA(p)
		if !w.threeD || c.R != 0 || c.G != 0 || c.B != 0 {
			return c, true
		}
	}
	return color.RGBA{0, 0, 0, 255}, false
}
