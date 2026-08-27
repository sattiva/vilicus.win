package discord

import (
	"sync"
	"time"
)


const (
	cdFastRate      = 5.0 / 10.0
	cdFastCap       = 5.0
	cdDangerRate    = 2.0 / 10.0
	cdDangerCap     = 2.0
	cooldownMaxIdle = 10 * time.Minute
)

type cdBucket struct {
	tokens float64
	last   time.Time
}

type Cooldowns struct {
	mu      sync.Mutex
	buckets map[string]*cdBucket
	stop    chan struct{}
}

func NewCooldowns() *Cooldowns {
	c := &Cooldowns{
		buckets: make(map[string]*cdBucket),
		stop:    make(chan struct{}),
	}
	go c.janitor()
	return c
}

func (c *Cooldowns) janitor() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			cut := time.Now().Add(-cooldownMaxIdle)
			c.mu.Lock()
			for k, b := range c.buckets {
				if b.last.Before(cut) {
					delete(c.buckets, k)
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *Cooldowns) Allow(key, class string) bool {
	rate, capF := cdFastRate, cdFastCap
	if class == "danger" {
		rate, capF = cdDangerRate, cdDangerCap
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	b, ok := c.buckets[key]
	if !ok {
		b = &cdBucket{tokens: capF, last: now}
		c.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * rate
	if b.tokens > capF {
		b.tokens = capF
	}
	b.last = now

	if b.tokens < 1.0 {
		return false
	}
	b.tokens -= 1.0
	return true
}

func (c *Cooldowns) Stop() {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
}

