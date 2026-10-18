package session

import "time"

const (
	// DefaultFrameRate is the publish cap. The step loop used to publish
	// on a fixed 30ms ticker (about 33 fps). 24 fps is the runtime default
	// so a browser can skip intermediate snapshots without following every tick.
	DefaultFrameRate = 24
	MinFrameRate     = 1
	MaxFrameRate     = 120
)

func clampFrameRate(fps int) int {
	if fps < MinFrameRate {
		return MinFrameRate
	}
	if fps > MaxFrameRate {
		return MaxFrameRate
	}
	return fps
}

func frameInterval(fps int) time.Duration {
	if fps == 0 {
		fps = DefaultFrameRate
	}
	return time.Second / time.Duration(clampFrameRate(fps))
}

// framePublisher counts model steps that were never published.
// publish keeps the latest snapshot only: it does not queue and it does
// not block the step loop.
type framePublisher struct {
	pending      int
	published    int
	dropped      int
	droppedSlow  int
	windowStart  time.Time
	windowCount  int
	publishedFPS float64
	lastPublish  time.Time
}

func (p *framePublisher) produce(steps int) {
	if steps <= 0 {
		return
	}
	p.pending += steps
}

// due reports that the cap interval has elapsed and a snapshot is waiting.
func (p *framePublisher) due(now time.Time, interval time.Duration) bool {
	if p.pending <= 0 {
		return false
	}
	if p.lastPublish.IsZero() {
		return true
	}
	return !now.Before(p.lastPublish.Add(interval))
}

// publish records one send of the latest snapshot. Every pending step
// except that one was skipped by the cap.
func (p *framePublisher) publish(now time.Time) bool {
	if p.pending <= 0 {
		return false
	}
	if p.pending > 1 {
		p.dropped += p.pending - 1
	}
	p.pending = 0
	p.published++
	p.lastPublish = now
	p.noteFPS(now)
	return true
}

func (p *framePublisher) noteSlow(n int) {
	if n > 0 {
		p.droppedSlow += n
	}
}

func (p *framePublisher) noteFPS(now time.Time) {
	if p.windowStart.IsZero() || now.Sub(p.windowStart) >= time.Second {
		if !p.windowStart.IsZero() {
			span := now.Sub(p.windowStart).Seconds()
			if span > 0 {
				p.publishedFPS = float64(p.windowCount) / span
			}
		}
		p.windowStart = now
		p.windowCount = 0
	}
	p.windowCount++
}

func (p *framePublisher) reset() {
	*p = framePublisher{}
}
