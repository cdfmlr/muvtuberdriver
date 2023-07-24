package sayer

import (
	"context"
	"errors"
	"fmt"
	"muvtuberdriver/audio"
	"muvtuberdriver/live2d"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cdfmlr/ellipsis"
	"golang.org/x/exp/slog"

	musayerapi "github.com/murchinroom/sayerapigo"
)

type LipsyncStrategy string

const (
	LipsyncStrategyNone         LipsyncStrategy = "none" // default: no lipsync
	LipsyncStrategyKeepMotion   LipsyncStrategy = "keep_motion"
	LipsyncStrategyAudioAnalyze LipsyncStrategy = "audio_analyze"
)

const (
	defaultTtsRole         = "default"
	defaultLipsyncStrategy = LipsyncStrategyNone
)

// lipsyncSayer is a Sayer implementation
// that do tts & audio playback jobs
// with blocking, mutexing and live2d lips syncing.
//
// 其实基本上就是重写了之前的 allInOneSayer + internalSayer，感觉职责更重了？但是懒得拆了。。
// 兼容了之前的 allInOneSayer 的全部功能。
type lipsyncSayer struct {
	// dependencies

	textAudioConverter musayerapi.Sayer
	playbackController audio.Controller
	live2dDriver       live2d.Driver

	// config

	lipsyncStrategy LipsyncStrategy
	ttsRole         string

	// internal state

	saying sync.Mutex
	fails  atomic.Int32
}

func NewLipsyncSayer(textAudioConverterAddr string,
	playbackController audio.Controller, live2dDriver live2d.Driver,
	opts ...LipsyncSayerOption) Sayer {

	pool, err := musayerapi.NewSayerClientPool(textAudioConverterAddr, 8)
	if err != nil {
		panic(err) // NewSayerClientPool should not fail
	}

	lss := &lipsyncSayer{
		textAudioConverter: pool,
		playbackController: playbackController,
		live2dDriver:       live2dDriver,
	}

	for _, opt := range opts {
		opt(lss)
	}
	if lss.ttsRole == "" {
		lss.ttsRole = defaultTtsRole
	}
	if lss.lipsyncStrategy == "" {
		lss.lipsyncStrategy = defaultLipsyncStrategy
	}

	return lss
}

type LipsyncSayerOption func(*lipsyncSayer)

func WithLipsyncStrategy(strategy LipsyncStrategy) LipsyncSayerOption {
	return func(s *lipsyncSayer) {
		s.lipsyncStrategy = strategy
	}
}

func WithTtsRole(role string) LipsyncSayerOption {
	return func(s *lipsyncSayer) {
		s.ttsRole = role
	}
}

// Say implements Sayer.Say.
// Say is blocking, mutexing and live2d lips syncing.
func (s *lipsyncSayer) Say(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	s.saying.Lock()
	defer s.saying.Unlock()

	return s.say(text)
}

// say do the core job (unsafely, blocking):
//
//	text -> audio -> playback & lipsync -> wait
func (s *lipsyncSayer) say(text string) error {
	logger := slog.With("text", ellipsis.Centering(text, 15))

	if s.lipsyncStrategy == LipsyncStrategyKeepMotion { // sent earlier: looks more synchronous
		s.live2dDriver.Live2dToMotion("flick_head") // 准备张嘴说话
		defer s.live2dDriver.Live2dToMotion("idle") // 说完闭嘴
	}

	// text -> audio

	format, audioContent, err := s.textToAudio(text)
	if err != nil {
		logger.Warn("[lipsyncSayer] say failed (textToAudio)", "err", err)
		return err
	}

	// audio -> track

	track := s.audioToTrack(format, audioContent)

	// blockingPlayback

	if s.lipsyncStrategy == LipsyncStrategyAudioAnalyze {
		err := s.live2dDriver.Live2dSpeak(audioContent, "", "") // TODO: expression, motion
		if err != nil {
			logger.Warn("[lipsyncSayer] Live2dSpeak failed (LipsyncStrategyAudioAnalyze)",
				"err", err, "falling-back-to", "LipsyncStrategyKeepMotion")

			// fallback to LipsyncStrategyKeepMotion
			s.live2dDriver.Live2dToMotion("flick_head")
			defer s.live2dDriver.Live2dToMotion("idle")
		}
	}

	if err := s.blockingPlayback(track, logger); err != nil {
		logger.Error("[lipsyncSayer] say failed (playback)", "err", err)
		s.fails.Add(1)
		return err
	} else {
		s.fails.Store(0)
	}

	return nil
}

// shouldPlayAt returns the PlayAt value according
// to the lostConsistency value.
func (s *lipsyncSayer) shouldPlayAt() audio.PlayAt {
	fails := s.fails.Load()
	switch {
	case fails > 3:
		return audio.PlayAtResetNow
	case fails > 0:
		return audio.PlayAtResetNext
	}
	return audio.PlayAtNext
}

// textToAudio converts text to audio via RPC.
func (s *lipsyncSayer) textToAudio(text string) (format string, audio []byte, err error) {
	return s.textAudioConverter.Say(s.ttsRole, text)
}

// audioToTrack converts audio to audio.Track locally.
func (s *lipsyncSayer) audioToTrack(format string, audioContent []byte) *audio.Track {
	track := s.playbackController.AudioToTrack(format, audioContent)
	track.PlayMode = string(s.shouldPlayAt())

	return track
}

// blockingPlayback plays the track by audioview and wait for the end of playback.
//
// packing the two functions into one method is for the convenience of lipsyncSayer.say().
func (s *lipsyncSayer) blockingPlayback(track *audio.Track, logger *slog.Logger) error {
	if len(track.ID) == 0 {
		return errors.New("track.ID is empty")
	}
	logger = logger.With("trackID", ellipsis.Centering(track.ID, 9)).With("func", "blockingPlayback")

	if err := s.playbackController.PlayVocal(track); err != nil {
		return err
	}

	return s.waitPlaying(track.ID, logger)
}

// waitPlaying blocks until the track is played (end) or error occurred (timeout, etc.).
//
// Waiting start for 30 seconds and end for 300 seconds:
//   - !start && !end => err: timeout (not started)
//   - start && !end => err: timeout (not ended)
//   - !start &&  end => err: ok (ended, but start report lost)
//   - start &&  end => err: ok (ended normally)
//
// if any error occurred (start or end), an error will be returned immediately.
func (s *lipsyncSayer) waitPlaying(trackID string, logger *slog.Logger) error {
	// here the ctx{Start, End} are used to control the timeout.
	ctxStart, cancelStart := context.WithTimeout(context.Background(), time.Second*30)
	defer cancelStart()

	ctxEnd, cancelEnd := context.WithTimeout(context.Background(), time.Second*300)
	defer cancelEnd()

	// there is at most one msg sent to each of the channels.
	chStart := make(chan error, 1)
	chEnd := make(chan error, 1)

	go func() {
		chStart <- s.playbackController.Wait(ctxStart, audio.ReportStart(trackID))
	}()

	go func() {
		chEnd <- s.playbackController.Wait(ctxEnd, audio.ReportEnd(trackID))
	}()

	for {
		select {
		case err := <-chEnd:
			if err != nil {
				return fmt.Errorf("wait END report from audioview failed: %w", err)
			}
			return nil // success
		case err := <-chStart:
			if err != nil { // quick fail
				return fmt.Errorf("wait START report from audioview failed: %w", err)
			}
			continue // wait for END report
		}
		// p.s. an oral proof of the termination:
		// there is always an err from chEnd:
		//  - nil if success
		//  - or an error if failed or context timeout
		// so the select above will never block forever.
	}
	// unreachable
}
