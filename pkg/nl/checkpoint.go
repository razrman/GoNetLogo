package nl

import "time"

// DefaultYieldInterval is how often a long button lets the UI publish
// a frame and observe stop, speed, and the frame-rate cap.
const DefaultYieldInterval = 50 * time.Millisecond

// RequestHalt asks the running button to stop at the next checkpoint.
// It does not take the world lock, so a UI thread is not stuck behind a
// generation that is still inside RunCompiled.
func (vm *VM) RequestHalt() {
	if vm != nil {
		vm.halt.Store(true)
	}
}

// ClearHalt forgets a previous stop so the next button can run.
func (vm *VM) ClearHalt() {
	if vm != nil {
		vm.halt.Store(false)
	}
}

// Halted reports whether RequestHalt has been called since ClearHalt.
func (vm *VM) Halted() bool {
	return vm != nil && vm.halt.Load()
}

// SetYield installs a callback invoked on the frame cadence while a button
// runs. The callback runs with the world lock released and must not call
// back into the VM. ClearYield removes it. Passing a non-positive interval
// uses DefaultYieldInterval.
func (vm *VM) SetYield(fn func(), every time.Duration) {
	if every <= 0 {
		every = DefaultYieldInterval
	}
	vm.yield = fn
	vm.yieldEvery = every
	vm.lastYield = time.Now()
}

// ClearYield removes the frame callback. A later button does not publish
// through a session that has moved on.
func (vm *VM) ClearYield() {
	if vm != nil {
		vm.yield = nil
	}
}

// checkpoint polls halt and, on the frame cadence, lets the UI publish.
// The caller holds World.mu. Halt unwinds to RunCompiled as a stopped button.
func (vm *VM) checkpoint() {
	if vm.external > 0 {
		return
	}
	if vm.halt.Load() {
		panic(interruptPanic{})
	}
	if vm.yield == nil {
		return
	}
	vm.stepN++
	if vm.stepN&15 != 0 {
		return
	}
	every := vm.yieldEvery
	if every <= 0 {
		every = DefaultYieldInterval
	}
	now := time.Now()
	if now.Sub(vm.lastYield) < every {
		return
	}
	vm.lastYield = now
	// The published view is a copy taken when dirty is set. A long button
	// used to publish only when it returned, so the desktop painter kept
	// the frame from before go. Mark it stale before dropping the lock.
	vm.World.dirty.Store(true)
	vm.World.mu.Unlock()
	func() {
		defer vm.World.mu.Lock()
		vm.yield()
	}()
	if vm.halt.Load() {
		panic(interruptPanic{})
	}
}
