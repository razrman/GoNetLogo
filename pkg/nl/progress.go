package nl

// Progress is a point-in-time sample of a running model. The desktop and
// the server log it on a timer. It is not a frame and it does not copy
// turtles or plot points.
type Progress struct {
	Ticks      float64
	Started    bool
	Turtles    int
	Patches    int
	Drawing    int
	PlotPoints int
	OutputLen  int
	RNGDraws   uint64
}

// Progress samples the world. It takes the world lock.
func (vm *VM) Progress() Progress {
	if vm == nil || vm.World == nil {
		return Progress{}
	}
	var p Progress
	vm.DoLocked(func() { p = vm.progressLocked() })
	return p
}

// progressLocked reads counters. The caller holds the world lock.
func (vm *VM) progressLocked() Progress {
	w := vm.World
	p := Progress{
		Patches:    w.width * w.height,
		Drawing:    w.drawing.n,
		RNGDraws:   w.RNGDraws(),
		OutputLen:  outputLen(vm),
		PlotPoints: plotPointCount(vm),
	}
	for _, t := range w.turtles {
		if t != nil && !t.dead {
			p.Turtles++
		}
	}
	p.Started = w.ticksStarted
	if p.Started {
		if n, err := w.Ticks(); err == nil {
			p.Ticks = n
		}
	}
	return p
}

func outputLen(vm *VM) int {
	n := 0
	for i, line := range vm.outputLines {
		if i > 0 {
			n++
		}
		n += len(line)
	}
	if vm.outputPartial != "" {
		if n > 0 {
			n++
		}
		n += len(vm.outputPartial)
	}
	return n
}

func plotPointCount(vm *VM) int {
	if vm == nil || vm.plots == nil {
		return 0
	}
	n := 0
	for _, p := range vm.plots.plots {
		for _, pen := range p.pens {
			n += len(pen.points)
		}
	}
	return n
}
