package nl

import (
	"fmt"
	"math"
	"strings"
)

// NetLogo 3D dialect. A program parsed with Parse3D sees these tables on
// top of the 2D ones. A 2D program never sees them, so 2D models keep
// "nothing named SETXYZ" and every 2D arity.

// reporters3D are 3D reporters with arguments, and 2D reporters whose
// arity changes in 3D.
var reporters3D = map[string]primSpec{
	"patch":                               {argc: 3, prec: precNormal},
	"patch-at":                            {argc: 3, prec: precNormal},
	"turtles-at":                          {argc: 3, prec: precNormal},
	"distancexyz":                         {argc: 3, prec: precNormal},
	"distancexyz-nowrap":                  {argc: 3, prec: precNormal},
	"towards-pitch":                       {argc: 1, prec: precNormal},
	"towards-pitch-nowrap":                {argc: 1, prec: precNormal},
	"towards-pitch-xyz":                   {argc: 3, prec: precNormal},
	"towards-pitch-xyz-nowrap":            {argc: 3, prec: precNormal},
	"patch-at-heading-pitch-and-distance": {argc: 3, prec: precNormal},
}

// zeroArgReporters3D are the 3D zero-argument reporters.
var zeroArgReporters3D = map[string]bool{
	"dz":           true,
	"min-pzcor":    true,
	"max-pzcor":    true,
	"world-depth":  true,
	"random-zcor":  true,
	"random-pzcor": true,
	"neighbors6":   true,
	"link-pitch":   true,
	"link-length":  true,
	"__oxcor":      true,
	"__oycor":      true,
	"__ozcor":      true,
}

// commands3D are 3D commands, and 2D commands whose arity changes in 3D.
var commands3D = map[string]primSpec{
	"setxyz":         {argc: 3},
	"facexyz":        {argc: 3},
	"tilt-up":        {argc: 1},
	"tilt-down":      {argc: 1},
	"roll-left":      {argc: 1},
	"roll-right":     {argc: 1},
	"orbit-up":       {argc: 1},
	"orbit-down":     {argc: 1},
	"orbit-left":     {argc: 1},
	"orbit-right":    {argc: 1},
	"zoom":           {argc: 1},
	"resize-world":   {argc: 6},
	"load-shapes-3d": {argc: 1},
}

// isZeroArgReporter reports whether name is a zero-argument reporter in
// the dialect.
func isZeroArgReporter(name string, is3D bool) bool {
	return zeroArgReporters[name] || (is3D && zeroArgReporters3D[name])
}

// is3DCommand reports whether name is a command only in the 3D dialect.
func is3DCommand(name string, prog *Program) bool {
	if prog == nil || !prog.Is3D {
		return false
	}
	_, ok := commands3D[name]
	return ok
}

func nums(args []Value, what string) ([]float64, error) {
	out := make([]float64, len(args))
	for i, a := range args {
		n, err := a.Num()
		if err != nil {
			return nil, fmt.Errorf("%s expected a number, got %s", what, a.TypeName())
		}
		out[i] = n
	}
	return out, nil
}

// point3Of is an agent's position. A patch is at its center.
func point3Of(a Agent) (float64, float64, float64, error) {
	switch a := a.(type) {
	case *Turtle:
		return a.X, a.Y, a.Z, nil
	case *Patch:
		return float64(a.PX), float64(a.PY), float64(a.PZ), nil
	default:
		return 0, 0, 0, fmt.Errorf("expected a turtle or patch")
	}
}

func (vm *VM) selfPoint3() (float64, float64, float64, error) {
	if vm.self == nil {
		return 0, 0, 0, fmt.Errorf("this reporter can only be used by a turtle or patch")
	}
	return point3Of(vm.self)
}

func (vm *VM) agentPoint3(v Value) (float64, float64, float64, error) {
	if v.K != KindAgent || v.A == nil {
		return 0, 0, 0, fmt.Errorf("expected an agent")
	}
	if v.A.IsDead() {
		return 0, 0, 0, fmt.Errorf("that agent is dead")
	}
	return point3Of(v.A)
}

// patchAtOffset3 is the patch at an offset from the caller. The observer
// measures from the origin. A 2D world ignores dz.
func (vm *VM) patchAtOffset3(dx, dy, dz float64) *Patch {
	w := vm.World
	if !w.threeD {
		return vm.patchAtOffset(dx, dy)
	}
	switch a := vm.self.(type) {
	case *Turtle:
		return w.PatchAt3(a.X+dx, a.Y+dy, a.Z+dz)
	case *Patch:
		return w.PatchAt3(float64(a.PX)+dx, float64(a.PY)+dy, float64(a.PZ)+dz)
	default:
		return w.PatchAt3(dx, dy, dz)
	}
}

func patchValue(p *Patch) Value {
	if p == nil {
		return Nobody()
	}
	return AgentVal(p)
}

// turn is rt/lt. In 3D it turns about the turtle's up vector.
func (vm *VM) turn(t *Turtle, delta float64) error {
	if vm.World.threeD {
		return vm.World.TurnRight3D(t, delta)
	}
	return vm.World.Turn(t, delta)
}

// primCmd3D runs the commands that are new or different in a 3D world.
// handled is false for every other command, which then takes the 2D path.
func (vm *VM) primCmd3D(name string, args []Value, pos Pos) (handled bool, err error) {
	w := vm.World
	switch name {
	case "setxyz":
		n, err := nums(args, "setxyz")
		if err != nil {
			return true, err
		}
		if vm.self == nil {
			// The observer form moves the camera.
			w.obsX, w.obsY, w.obsZ = n[0], n[1], n[2]
			return true, nil
		}
		return true, w.MoveTo3(vm.requireTurtle(pos), n[0], n[1], n[2])
	case "facexyz":
		n, err := nums(args, "facexyz")
		if err != nil {
			return true, err
		}
		if vm.self == nil {
			// Observer camera orientation is not modeled; see orbit-*.
			return true, nil
		}
		return true, w.FaceXYZ(vm.requireTurtle(pos), n[0], n[1], n[2], true)
	case "face":
		if vm.self == nil {
			// The observer form points the camera, which is not modeled.
			if args[0].K != KindAgent || args[0].A == nil || args[0].A.IsDead() {
				return true, fmt.Errorf("face expected an agent")
			}
			return true, nil
		}
		t := vm.requireTurtle(pos)
		x, y, z, err := vm.agentPoint3(args[0])
		if err != nil {
			return true, err
		}
		return true, w.FaceXYZ(t, x, y, z, true)
	case "home":
		if vm.self == nil {
			w.homeObserver()
			return true, nil
		}
		return true, w.MoveTo3(vm.requireTurtle(pos), 0, 0, 0)
	case "move-to":
		t := vm.requireTurtle(pos)
		target := args[0]
		if target.K == KindNobody || (target.K == KindAgent && (target.A == nil || target.A.IsDead())) {
			return true, fmt.Errorf("that agent is dead")
		}
		if target.K != KindAgent {
			return true, fmt.Errorf("move-to expected a turtle or patch")
		}
		if _, ok := target.A.(*Link); ok {
			return true, fmt.Errorf("you can't move-to a link")
		}
		x, y, z, err := point3Of(target.A)
		if err != nil {
			return true, err
		}
		return true, w.MoveTo3(t, x, y, z)
	case "rt", "right", "lt", "left":
		n, err := args[0].Num()
		if err != nil {
			return true, err
		}
		if name == "lt" || name == "left" {
			n = -n
		}
		return true, w.TurnRight3D(vm.requireTurtle(pos), n)
	case "tilt-up", "tilt-down":
		n, err := args[0].Num()
		if err != nil {
			return true, err
		}
		if name == "tilt-down" {
			n = -n
		}
		w.TiltUp(vm.requireTurtle(pos), n)
		return true, nil
	case "roll-right", "roll-left":
		n, err := args[0].Num()
		if err != nil {
			return true, err
		}
		if name == "roll-left" {
			n = -n
		}
		w.RollRight(vm.requireTurtle(pos), n)
		return true, nil
	case "orbit-up", "orbit-down", "orbit-left", "orbit-right", "zoom":
		// Camera-only observer commands. GoNetLogo does not model the 3D
		// camera orientation, and a headless run has no view, so these
		// check their input and do nothing.
		vm.requireObserver(pos, name)
		if _, err := args[0].Num(); err != nil {
			return true, fmt.Errorf("%s expected a number", name)
		}
		return true, nil
	case "load-shapes-3d":
		// 3D shapes only change how turtles are drawn by a 3D renderer,
		// which GoNetLogo does not have yet. Turtle shape names are not
		// checked against a library, so models keep running.
		if args[0].K != KindString {
			return true, fmt.Errorf("load-shapes-3d expected a string")
		}
		return true, nil
	case "resize-world":
		vm.requireObserver(pos, "resize-world")
		b := make([]int, 6)
		for i := range b {
			n, err := asInt(args[i], "resize-world")
			if err != nil {
				return true, err
			}
			b[i] = n
		}
		if err := ValidWorld3D(b[0], b[1], b[2], b[3], b[4], b[5]); err != nil {
			return true, err
		}
		w.Resize3(b[0], b[1], b[2], b[3], b[4], b[5])
		return true, nil
	case "reset-perspective", "rp":
		// NetLogo 3D's reset-perspective also sends the camera home. The
		// 2D path then clears watch/follow/ride.
		w.homeObserver()
		return false, nil
	}
	return false, nil
}

// evalReporter3D runs reporters that are new or different in a 3D world.
func (vm *VM) evalReporter3D(name string, args []Value, pos Pos) (Value, bool, error) {
	w := vm.World
	switch name {
	case "patch":
		if len(args) != 3 {
			return Value{}, false, nil
		}
		n, err := nums(args, "patch")
		if err != nil {
			return Value{}, true, err
		}
		for _, c := range n {
			if math.IsNaN(c) || math.IsInf(c, 0) {
				return Value{}, true, fmt.Errorf("patch expected a valid coordinate")
			}
		}
		p := w.PatchAtPcor3(int(math.Round(n[0])), int(math.Round(n[1])), int(math.Round(n[2])))
		return patchValue(p), true, nil
	case "patch-at":
		if len(args) != 3 {
			return Value{}, false, nil
		}
		n, err := nums(args, "patch-at")
		if err != nil {
			return Value{}, true, err
		}
		return patchValue(vm.patchAtOffset3(n[0], n[1], n[2])), true, nil
	case "turtles-at":
		if len(args) != 3 {
			return Value{}, false, nil
		}
		n, err := nums(args, "turtles-at")
		if err != nil {
			return Value{}, true, err
		}
		p := vm.patchAtOffset3(n[0], n[1], n[2])
		if p == nil {
			return SetVal(turtleSet(nil)), true, nil
		}
		return SetVal(turtleSet(w.TurtlesHere(p))), true, nil
	case "distance":
		x, y, z, err := vm.agentPoint3(args[0])
		if err != nil {
			return Value{}, true, err
		}
		sx, sy, sz, err := vm.selfPoint3()
		if err != nil {
			return Value{}, true, err
		}
		return Num(w.Distance3(sx, sy, sz, x, y, z)), true, nil
	case "distancexyz", "distancexyz-nowrap":
		n, err := nums(args, name)
		if err != nil {
			return Value{}, true, err
		}
		sx, sy, sz, err := vm.selfPoint3()
		if err != nil {
			return Value{}, true, err
		}
		if name == "distancexyz-nowrap" {
			return Num(distanceNoWrap(sx, sy, sz, n[0], n[1], n[2])), true, nil
		}
		return Num(w.Distance3(sx, sy, sz, n[0], n[1], n[2])), true, nil
	case "towards-pitch", "towards-pitch-nowrap":
		x, y, z, err := vm.agentPoint3(args[0])
		if err != nil {
			return Value{}, true, err
		}
		sx, sy, sz, err := vm.selfPoint3()
		if err != nil {
			return Value{}, true, err
		}
		p, err := w.PitchTowards(sx, sy, sz, x, y, z, name == "towards-pitch")
		return Num(p), true, err
	case "towards-pitch-xyz", "towards-pitch-xyz-nowrap":
		n, err := nums(args, name)
		if err != nil {
			return Value{}, true, err
		}
		sx, sy, sz, err := vm.selfPoint3()
		if err != nil {
			return Value{}, true, err
		}
		p, err := w.PitchTowards(sx, sy, sz, n[0], n[1], n[2], name == "towards-pitch-xyz")
		return Num(p), true, err
	case "patch-ahead":
		d, err := args[0].Num()
		if err != nil {
			return Value{}, true, err
		}
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, true, fmt.Errorf("patch-ahead can only be used by a turtle")
		}
		dx, dy, dz := orientDelta(t.Heading, t.Pitch, d)
		return patchValue(w.PatchAt3(t.X+dx, t.Y+dy, t.Z+dz)), true, nil
	case "patch-left-and-ahead", "patch-right-and-ahead":
		n, err := nums(args, name)
		if err != nil {
			return Value{}, true, err
		}
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, true, fmt.Errorf("%s can only be used by a turtle", name)
		}
		ang := n[0]
		if name == "patch-left-and-ahead" {
			ang = -ang
		}
		// The direction the turtle would face after right ang, which is a
		// turn about its up vector.
		f, u, _ := orientFrame(t.Heading, t.Pitch, t.Roll)
		f = rotateAbout(f, u, -ang)
		return patchValue(w.PatchAt3(t.X+f[0]*n[1], t.Y+f[1]*n[1], t.Z+f[2]*n[1])), true, nil
	case "patch-at-heading-and-distance":
		n, err := nums(args, name)
		if err != nil {
			return Value{}, true, err
		}
		sx, sy, sz, err := vm.selfPoint3()
		if err != nil {
			return Value{}, true, err
		}
		dx, dy := headingDelta(n[0], n[1])
		return patchValue(w.PatchAt3(sx+dx, sy+dy, sz)), true, nil
	case "patch-at-heading-pitch-and-distance":
		n, err := nums(args, name)
		if err != nil {
			return Value{}, true, err
		}
		sx, sy, sz, err := vm.selfPoint3()
		if err != nil {
			return Value{}, true, err
		}
		dx, dy, dz := orientDelta(n[0], n[1], n[2])
		return patchValue(w.PatchAt3(sx+dx, sy+dy, sz+dz)), true, nil
	}
	return Value{}, false, nil
}

// inCone3D is in-cone in a 3D world: agents within dist whose direction
// is within angle/2 of the turtle's forward vector (heading and pitch).
func (vm *VM) inCone3D(self *Turtle, ags *AgentSet, dist, angle float64) (Value, error) {
	w := vm.World
	f, _, _ := unrolledFrame(self.Heading, self.Pitch)
	cosHalf := math.Cos(angle / 2 * math.Pi / 180)
	var keep []Agent
	for _, a := range ags.Alive() {
		x, y, z, err := point3Of(a)
		if err != nil {
			return Value{}, err
		}
		dx, dy, dz := w.delta3(self.X, self.Y, self.Z, x, y, z, true)
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d > dist {
			continue
		}
		if d == 0 || angle >= 360 {
			keep = append(keep, a)
			continue
		}
		if (f[0]*dx+f[1]*dy+f[2]*dz)/d >= cosHalf-1e-12 {
			keep = append(keep, a)
		}
	}
	return SetVal(&AgentSet{Kind: ags.Kind, Breed: ags.Breed, Agents: keep}), nil
}

// evalZero3D is the 3D zero-argument reporters and the 3D forms of dx,
// dy, and neighbors.
func (vm *VM) evalZero3D(name string, pos Pos) (Value, bool, error) {
	w := vm.World
	if !w.threeD {
		if zeroArgReporters3D[name] {
			return Value{}, true, errorAt(pos, "%s is only available in NetLogo 3D", strings.ToUpper(name))
		}
		return Value{}, false, nil
	}
	switch name {
	case "dx", "dy", "dz":
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, true, errorAt(pos, "%s can only be used by a turtle", name)
		}
		dx, dy, dz := orientDelta(t.Heading, t.Pitch, 1)
		switch name {
		case "dx":
			return Num(dx), true, nil
		case "dy":
			return Num(dy), true, nil
		}
		return Num(dz), true, nil
	case "neighbors6":
		p := vm.patchUnderSelf()
		if p == nil {
			return Value{}, true, errorAt(pos, "neighbors6 can only be used by a turtle or patch")
		}
		return SetVal(patchSet(w.Neighbors6(p))), true, nil
	case "min-pzcor":
		return Num(float64(w.MinPzcor)), true, nil
	case "max-pzcor":
		return Num(float64(w.MaxPzcor)), true, nil
	case "world-depth":
		return Num(float64(w.depth)), true, nil
	case "random-zcor":
		return Num(w.RandomZcor()), true, nil
	case "random-pzcor":
		return Num(float64(w.RandomPzcor())), true, nil
	case "link-pitch", "link-length":
		l, ok := vm.self.(*Link)
		if !ok {
			return Value{}, true, errorAt(pos, "%s can only be used by a link", name)
		}
		a, b := l.End1, l.End2
		if name == "link-length" {
			return Num(w.Distance3(a.X, a.Y, a.Z, b.X, b.Y, b.Z)), true, nil
		}
		p, err := w.PitchTowards(a.X, a.Y, a.Z, b.X, b.Y, b.Z, true)
		if err != nil {
			return Value{}, true, errorAt(pos, "%s", err.Error())
		}
		return Num(p), true, nil
	case "__oxcor":
		return Num(w.obsX), true, nil
	case "__oycor":
		return Num(w.obsY), true, nil
	case "__ozcor":
		return Num(w.obsZ), true, nil
	}
	return Value{}, false, nil
}
