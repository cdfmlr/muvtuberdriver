package sayer

import (
	"context"
	"errors"

	"muvtuberdriver/audio"
	"muvtuberdriver/live2d"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cdfmlr/ellipsis"
	"golang.org/x/exp/slog"
)

// Deprecated: use NewLipsyncSayer instead. To be removed in v0.5.0.
//
// allInOneSayer is the muggles' dreaming Sayer..
// it Say()s with blocking, mutexing and live2d lips syncing.
//
// I really dislike this. It's definitely not a good design.
// But the main function is 不堪重负了. I have to do decoupling in a hurry.
// I will refactor this later.
type allInOneSayer struct {
	sayer           internalSayer
	saying          sync.Mutex
	live2dDriver    live2d.Driver // for lips sync
	lostConsistency atomic.Int32  // have been giving up waiting audio start or end
}

func (s *allInOneSayer) Say(text string) error {
	logger := slog.With("text", ellipsis.Centering(text, 15))

	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	s.saying.Lock()
	defer s.saying.Unlock()

	s.live2dDriver.Live2dToMotion("flick_head") // 准备张嘴说话
	defer s.live2dDriver.Live2dToMotion("idle") // 说完闭嘴

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*300) // endTimeout
	defer cancel()

	fails := s.lostConsistency.Load()
	switch {
	case fails > 3:
		ctx = context.WithValue(ctx, "playAt", audio.PlayAtResetNow)
	case fails > 0:
		ctx = context.WithValue(ctx, "playAt", audio.PlayAtResetNext)
	}

	ch, err := s.sayer.Say(ctx, text)
	if err != nil {
		logger.Warn("[allInOneSayer] say failed (tts OR initial the audio blockingPlayback task)", "err", err)
		return err
	}
	started := false
	startTimeout := time.After(time.Second * 30)
	// ctx 还自带一个 300 秒的 endTimeout
	for {
		select {
		case r := <-ch:
			switch r {
			case audio.PlayStatusStart:
				logger.Info("[allInOneSayer] AudioPlayStatusStart")
				started = true
			case audio.PlayStatusEnd:
				logger.Info("[allInOneSayer] AudioPlayStatusEnd: Done!")
				s.lostConsistency.Store(0)

				return nil
			case audio.PlayStatusErr:
				logger.Warn("[allInOneSayer] AudioPlayStatusErr: Failed!")
				s.lostConsistency.Add(1)

				return errors.New("AudioPlayStatusErr")
			}
		case <-startTimeout:
			// 半天没开始说，干啥呢，直接认为出问题了，不说了。
			if !started {
				logger.Warn("[allInOneSayer] start playing audio timeout: Canceling...")
				cancel() // cancel 导致：会在清理完底层工作后，走上面的 AudioPlayStatusErr case 退出
				s.lostConsistency.Add(1)
			}
			// 已经开始了就等它说：再等 270 秒
		}
	}
	// never reach here
}

// Deprecated: use NewLipsyncSayer instead.
func NewAllInOneSayer(addr string, role string, audioController audio.Controller, live2dDriver live2d.Driver) Sayer {
	return &allInOneSayer{
		sayer:        new_sayer(addr, role, audioController),
		live2dDriver: live2dDriver,
	}
}
