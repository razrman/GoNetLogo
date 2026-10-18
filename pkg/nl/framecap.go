package nl

import "time"

const (
	// DefaultFrameRate is the publish cap. The step loop used to publish
	// on a fixed 30ms ticker (about 33 fps). 24 fps is the runtime default
	// so a view can skip intermediate snapshots without following every tick.
	DefaultFrameRate = 24
	MinFrameRate     = 1
	MaxFrameRate     = 120
)

// ClampFrameRate limits fps to 1..120.
func ClampFrameRate(fps int) int {
	if fps < MinFrameRate {
		return MinFrameRate
	}
	if fps > MaxFrameRate {
		return MaxFrameRate
	}
	return fps
}

// FrameInterval is the gap between published snapshots. An unset rate uses
// DefaultFrameRate so a zero value does not become 1 fps.
func FrameInterval(fps int) time.Duration {
	if fps == 0 {
		fps = DefaultFrameRate
	}
	return time.Second / time.Duration(ClampFrameRate(fps))
}

// FramePublisher counts model steps that were never published.
// Publish keeps the latest snapshot only: it does not queue and it does
// not block the step loop. Web and desktop share this counter.
type FramePublisher struct {
	pending      int
	published    int
	dropped      int
	droppedSlow  int
	windowStart  time.Time
	windowCount  int
	publishedFPS float64
	lastPublish  time.Time
}

// Produce records steps that have run since the last published snapshot.
func (p *FramePublisher) Produce(steps int) {
	if steps <= 0 {
		return
	}
	p.pending += steps
}

// Due reports that the cap interval has elapsed and a snapshot is waiting.
func (p *FramePublisher) Due(now time.Time, interval time.Duration) bool {
	if p.pending <= 0 {
		return false
	}
	if p.lastPublish.IsZero() {
		return true
	}
	return !now.Before(p.lastPublish.Add(interval))
}

// Publish records one send of the latest snapshot. Every pending step
// except that one was skipped by the cap. interval is the cap; a non-positive
// interval does not rate-limit (tests of the drop counter). A second caller
// in the same interval is refused, so a step loop and a timer cannot both publish.
func (p *FramePublisher) Publish(now time.Time, interval time.Duration) bool {
	if p.pending <= 0 {
		return false
	}
	if interval > 0 && !p.lastPublish.IsZero() && now.Before(p.lastPublish.Add(interval)) {
		return false
	}
	if p.pending > 1 {
		p.dropped += p.pending - 1
	}
	p.pending = 0
	p.published++
	p.lastPublish = now
	p.noteFPS(now, interval)
	return true
}

// NextWait is how long a timer should sleep after a publish attempt.
// A refused early fire waits out the rest of the interval instead of
// starting a new full period from the wrong phase.
func (p *FramePublisher) NextWait(now time.Time, interval time.Duration) time.Duration {
	if interval <= 0 {
		interval = time.Second / time.Duration(DefaultFrameRate)
	}
	if p.lastPublish.IsZero() {
		return interval
	}
	wait := interval - now.Sub(p.lastPublish)
	if wait <= 0 || wait > interval {
		return interval
	}
	return wait
}

// NoteSlow counts publishes a subscriber missed. Separate from cap drops.
func (p *FramePublisher) NoteSlow(n int) {
	if n > 0 {
		p.droppedSlow += n
	}
}

// Dropped is the number of steps skipped by the cap since Reset.
func (p *FramePublisher) Dropped() int { return p.dropped }

// DroppedSlow is the number of publishes a slow subscriber missed.
func (p *FramePublisher) DroppedSlow() int { return p.droppedSlow }

// PublishedFPS is the publish rate over about the last second, never above the cap.
func (p *FramePublisher) PublishedFPS() float64 { return p.publishedFPS }

func (p *FramePublisher) noteFPS(now time.Time, interval time.Duration) {
	if p.windowStart.IsZero() || now.Sub(p.windowStart) >= time.Second {
		if !p.windowStart.IsZero() {
			span := now.Sub(p.windowStart).Seconds()
			if span > 0 {
				fps := float64(p.windowCount) / span
				// Integer division makes time.Second/fps a hair under the
				// ideal interval, so a one-second window can count one extra
				// publish. The reported rate still must not exceed the cap.
				if interval > 0 {
					capFPS := float64(time.Second) / float64(interval)
					if fps > capFPS {
						fps = capFPS
					}
				}
				p.publishedFPS = fps
			}
		}
		p.windowStart = now
		p.windowCount = 0
	}
	p.windowCount++
}

// Reset clears the counters. Call it on setup and when a model is loaded.
func (p *FramePublisher) Reset() {
	*p = FramePublisher{}
}
