package chatbot

import (
	"log"
	"os"
	"sync/atomic"
	"time"
)

type Cooldown struct {
	coolingdown atomic.Bool
	Interval    time.Duration
}

var DefaultCooldownInterval = time.Second * 60

func (c *Cooldown) AccessWithCooldown() bool {
	if c.Interval == 0 {
		c.setupInterval()
	}

	if c.coolingdown.Load() {
		return false
	}

	c.coolingdown.Store(true)
	go func() {
		time.Sleep(c.Interval)
		c.coolingdown.Store(false)
	}()

	return true
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
			log.Printf("use cooldown interval from env: %v", interval)
			c.Interval = interval
		}
	} else {
		log.Printf("cooldown interval missing, use default: %v", DefaultCooldownInterval)
		c.Interval = DefaultCooldownInterval
	}
}
