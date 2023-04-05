package main

import (
	"context"
	"errors"
	"muvtuberdriver/musayerapi"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/exp/slog"
)

// Sayer is the simple sayer interface for muggles.
// Sayer does blocking & mutex Say().
type Sayer interface {
	// Say text.
	Say(text string) error
}

// allInOneSayer is the muggles' dreaming Sayer..
// it Say()s with blocking, mutexing and live2d lips syncing.
//
// I really dislike this. It's definitely not a good design.
// But the main function is 不堪重负了. I have to do decoupling in a hurry.
// I will refactor this later.
type allInOneSayer struct {
	sayer           internalSayer
	saying          sync.Mutex
	live2dDriver    Live2DDriver // for lips sync
	lostConsistency atomic.Int32 // have been giving up waiting audio start or end
}

func (s *allInOneSayer) Say(text string) error {
	defer slog.Info("say: done", "text", text)
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	s.saying.Lock()
	defer s.saying.Unlock()

	s.live2dDriver.live2dToMotion("flick_head") // 准备张嘴说话
	defer s.live2dDriver.live2dToMotion("idle") // 说完闭嘴

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*300) // endTimeout
	defer cancel()

	fails := s.lostConsistency.Load()
	switch {
	case fails > 3:
		ctx = context.WithValue(ctx, "playAt", PlayAtResetNow)
	case fails > 0:
		ctx = context.WithValue(ctx, "playAt", PlayAtResetNext)
	}

	ch, err := s.sayer.Say(ctx, text)
	if err != nil {
		slog.Warn("say failed", "err", err, "text", text)
		return err
	}
	started := false
	startTimeout := time.After(time.Second * 30)
	// ctx 还自带一个 300 秒的 endTimeout
	for {
		select {
		case r := <-ch:
			switch r {
			case AudioPlayStatusStart:
				started = true
			case AudioPlayStatusEnd:
				slog.Info("AudioPlayStatusEnd", "text", text)
				s.lostConsistency.Store(0)
				return nil
			case AudioPlayStatusErr:
				s.lostConsistency.Add(1)
				return errors.New("AudioPlayStatusErr")
			}
		case <-startTimeout:
			if !started {
				cancel() // 半天没开始说，干啥呢，直接认为出问题了，不说了。
				s.lostConsistency.Add(1)
			}
			// 已经开始了就等它说：再等 270 秒
		}
	}
	return nil
}

func NewAllInOneSayer(addr string, role string, audioController AudioController, live2dDriver Live2DDriver) Sayer {
	return &allInOneSayer{
		sayer:        new_sayer(addr, role, audioController),
		live2dDriver: live2dDriver,
	}
}

// internalSayer is non-blocking.
//
// that use musayerapi & AudioController to do tts & playback jobs.
type internalSayer interface {
	// Say is non-blocking.
	//
	// returned chan reports the status of the audio playing (start, end) and
	// it will be closed when the audioview finished playing the audio.
	// ctx is used to cancel the waiting (not the say job, the command always sent)
	Say(ctx context.Context, text string) (chan AudioPlayStatus, error)
}

// sayer is an internal sayer implementation.
//
// sayer calls:
//   - musayerapi.SayerClientPool.Say: to get the audio
//   - AudioController.PlayVocal: to play the audio
type sayer struct {
	cli              *musayerapi.SayerClientPool
	role             string
	auidioController AudioController // XXX: use chan instead of injecting AudioController
}

func new_sayer(addr string, role string, audioController AudioController) internalSayer {
	pool, err := musayerapi.NewSayerClientPool(addr, 8)
	if err != nil {
		panic(err) // NewSayerClientPool should not fail
	}
	return &sayer{
		cli:              pool,
		role:             role,
		auidioController: audioController,
	}
}

// Say wraps say() and do waiting jobs.
// Say is non-blocking. It do job in a goroutine and return immediately.
//
// the returned chan reports the status of the audio playing (start, end) and
// it will be closed when the audioview finished playing the audio.
// ctx is used to:
//   - cancel the waiting (not the say job, the command always sent)
//   - pass PlayAt value to the Track.
func (s *sayer) Say(ctx context.Context, text string) (chan AudioPlayStatus, error) {
	ch := make(chan AudioPlayStatus, 2)

	trackID, err := s.say(ctx, text)
	if err != nil {
		return nil, err
	}
	if len(trackID) == 0 {
		ch <- AudioPlayStatusStart
		ch <- AudioPlayStatusEnd
		return ch, nil
	}

	wg := sync.WaitGroup{} // to avoid panic: send on closed channel
	wg.Add(1)
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		err = s.auidioController.Wait(ctx, ReportStart(trackID))
		if err != nil {
			ch <- AudioPlayStatusErr
		} else {
			ch <- AudioPlayStatusStart
		}
		wg.Done()
	}()

	go func() {
		err = s.auidioController.Wait(ctx, ReportEnd(trackID))
		if err != nil {
			ch <- AudioPlayStatusErr
		} else {
			cancel() // end the goroutine above
			ch <- AudioPlayStatusEnd
		}
		wg.Wait()
		close(ch)
	}()

	return ch, nil
}

// say converts text to audio (via musayerapi) and play it (via AudioController).
func (s *sayer) say(ctx context.Context, text string) (trackID string, err error) {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return "", nil
	}

	// _, _, err := s.cli.Say("miku", text)
	// 草，这个 GitHub Copilot 老术力口了，有 role 字段还硬编码个 miku hhh

	format, audio, err := s.cli.Say(s.role, text)
	if err != nil {
		return "", err
	}

	track := s.auidioController.AudioToTrack(format, audio)

	if a, ok := ctx.Value("playAt").(AudioPlayAt); ok {
		track.PlayMode = string(a)
	}

	s.auidioController.PlayVocal(track)

	return track.ID, nil
}
