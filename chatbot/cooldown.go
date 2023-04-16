package chatbot

import (
	"log"
	"os"
	"sync/atomic"
	"time"
)

// Cooldown is a cooldown mechanism.
// It can be embedded in a struct to provide cooldown functionality.
//
// coolingdown (TryCooldown()) is atomic (thread-safe).
// But the lastUsed time (CooldownLeft()) is not guaranteed to be accurate. It's
// just a rough estimate.
type Cooldown struct {
	Interval time.Duration

	coolingdown atomic.Bool
	lastUsed    time.Time
}

var DefaultCooldownInterval = time.Second * 60

// TryCooldown returns true if the resource is available (not cooling down)
// and starts cooldown. Otherwise, it returns false.
func (c *Cooldown) TryCooldown() bool {
	if c.Interval == 0 {
		c.setupInterval()
	}

	if c.coolingdown.Load() {
		return false
	}

	c.lastUsed = time.Now()
	c.coolingdown.Store(true)
	go func() {
		time.Sleep(c.Interval)
		c.coolingdown.Store(false)
	}()

	return true
}

// CooldownLeftTime returns the time left for cooldown.
//
// If the resource is not cooling down, it returns 0.
// If the resource is cooling down, it returns the time left.
//
// The time left is not guaranteed to be accurate. It can even be negative.
// Don't use this method to determine if the resource is available.
// Use TryCooldown instead for that.
//
// This method should only be used for logging, debuging or providing user
// a rough estimate progress.
func (c *Cooldown) CooldownLeftTime() time.Duration {
	if c.Interval == 0 {
		c.setupInterval()
	}

	if !c.coolingdown.Load() {
		return 0
	}

	return (c.Interval - time.Since(c.lastUsed)).Round(time.Second / 10)
}

// setupInterval 从环境变量 COOLDOWN_INTERVAL 读取间隔时间，如果没有设置则使用默认值(DefaultCooldownInterval)。
func (c *Cooldown) setupInterval() {
	if c.Interval != 0 {
		return
	}
	envInterval := os.Getenv("COOLDOWN_INTERVAL")
	if envInterval != "" {
		interval, err := time.ParseDuration(envInterval)
		if err == nil {
			log.Printf("INFO [Cooldown] use cooldown interval from env: %v", interval)
			c.Interval = interval
			return
		}
	}
	log.Printf("WARN [Cooldown] cooldown interval missing, use default: %v", DefaultCooldownInterval)
	c.Interval = DefaultCooldownInterval
}
