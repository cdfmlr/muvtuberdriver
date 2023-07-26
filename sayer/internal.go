package sayer

import (
	"context"
	"muvtuberdriver/audio"
	"strings"
	"sync"

	"github.com/cdfmlr/ellipsis"
	musayerapi "github.com/murchinroom/sayerapigo"
	"golang.org/x/exp/slog"
)

// Deprecated: use lipsyncSayer instead. To be removed in v0.5.0.
//
// internalSayer is non-blocking.
//
// that use musayerapi & AudioController to do tts & blockingPlayback jobs.
type internalSayer interface {
	// Say is non-blocking.
	//
	// returned chan reports the status of the audio playing (start, end) and
	// it will be closed when the audioview finished playing the audio.
	// ctx is used to cancel the waiting (not the say job, the command always sent)
	Say(ctx context.Context, text string) (chan audio.PlayStatus, error)
}

// Deprecated: use lipsyncSayer instead. To be removed in v0.5.0.
//
// sayer is an internal sayer implementation.
//
// sayer calls:
//   - musayerapi.SayerClientPool.Say: to get the audio
//   - AudioController.PlayVocal: to play the audio
type sayer struct {
	cli              *musayerapi.SayerClientPool
	role             string
	auidioController audio.Controller // XXX: use chan instead of injecting AudioController
}

func new_sayer(addr string, role string, audioController audio.Controller) internalSayer {
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
func (s *sayer) Say(ctx context.Context, text string) (chan audio.PlayStatus, error) {
	logger := slog.With("text", ellipsis.Centering(text, 9))

	ch := make(chan audio.PlayStatus, 2)

	trackID, err := s.say(ctx, text)
	if err != nil {
		logger.Error("[sayer] say (TTS & PLAY) failed", "err", err)
		return nil, err
	}
	if len(trackID) == 0 {
		logger.Error("[sayer] say (TTS & PLAY) got an unexpected empty trackID with no err")
		ch <- audio.PlayStatusStart
		ch <- audio.PlayStatusEnd
		return ch, nil
	}

	logger = logger.With("trackID", trackID)

	wg := sync.WaitGroup{} // to avoid panic: send on closed channel
	wg.Add(1)
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		err = s.auidioController.Wait(ctx, audio.ReportStart(trackID))
		if err != nil {
			logger.Error("[sayer] wait START report from audioview failed", "err", err)
			ch <- audio.PlayStatusErr
		} else {
			logger.Info("[sayer] got audioview report: playing started")
			ch <- audio.PlayStatusStart
		}
		wg.Done()
	}()

	go func() {
		err = s.auidioController.Wait(ctx, audio.ReportEnd(trackID))
		if err != nil {
			logger.Error("[sayer] wait END report from audioview failed", "err", err)
			ch <- audio.PlayStatusErr
		} else {
			logger.Info("[sayer] got audioview report: playing ended")
			cancel() // end the goroutine above
			ch <- audio.PlayStatusEnd
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

	format, audioContent, err := s.cli.Say(s.role, text)
	if err != nil {
		return "", err
	}

	track := s.auidioController.AudioToTrack(format, audioContent)

	if a, ok := ctx.Value("playAt").(audio.PlayAt); ok {
		track.PlayMode = string(a)
	}

	s.auidioController.PlayVocal(track)

	return track.ID, nil
}
