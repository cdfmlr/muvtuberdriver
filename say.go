package main

import (
	"context"
	"muvtuberdriver/musayerapi"
	"strings"
)

type Sayer interface {
	// Say is non-blocking.
	//
	// returned chan reports the status of the audio playing (start, end) and
	// it will be closed when the audioview finished playing the audio.
	// ctx is used to cancel the waiting (not the say job, the command always sent)
	Say(ctx context.Context, text string) (chan AudioPlayStatus, error)
}

// sayer calls:
//   - musayerapi.SayerClientPool.Say: to get the audio
//   - AudioController.PlayVocal: to play the audio
type sayer struct {
	cli              *musayerapi.SayerClientPool
	role             string
	auidioController AudioController // XXX: use chan instead of injecting AudioController
}

func NewSayer(addr string, role string, audioController AudioController) Sayer {
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
func (s *sayer) Say(ctx context.Context, text string) (chan AudioPlayStatus, error) {
	ch := make(chan AudioPlayStatus, 2)

	trackID, err := s.say(text)
	if err != nil {
		return nil, err
	}
	if len(trackID) == 0 {
		ch <- StatusStart
		ch <- StatusEnd
		return ch, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		s.auidioController.Wait(ctx, ReportStart(trackID))
		ch <- StatusStart
	}()

	go func() {
		s.auidioController.Wait(ctx, ReportEnd(trackID))
		cancel() // end the goroutine above
		ch <- StatusEnd
		close(ch)
	}()

	return ch, nil
}

// say converts text to audio (via musayerapi) and play it (via AudioController).
func (s *sayer) say(text string) (trackID string, err error) {
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
	s.auidioController.PlayVocal(track)

	return track.ID, nil
}
