package main

import (
	"errors"
	"log"
	"os/exec"
	"strings"
	"time"
)

type Sayer interface {
	Say(text string) error
}

// sayer is a caller of the SAY(1) command on macOS.
type sayer struct {
	voice       string // Specify the voice to be used.
	rate        string // Speech rate to be used, in words per minute.
	audioDevice string // Specify, by ID or name prefix, an audio device to be used to play the audio
}

func NewSayer(o ...SayerOption) Sayer {
	s := &sayer{}
	for _, opt := range o {
		opt(s)
	}
	return s
}

// SayerOption configures a sayer.
// Available options are WithVoice, WithRate and WithAudioDevice.
type SayerOption func(s *sayer)

func WithVoice(voice string) SayerOption {
	return func(s *sayer) {
		s.voice = voice
	}
}

func WithRate(rate string) SayerOption {
	return func(s *sayer) {
		s.rate = rate
	}

}

func WithAudioDevice(audioDevice string) SayerOption {
	return func(s *sayer) {
		s.audioDevice = audioDevice
	}
}

func (s *sayer) say(text string) error {
	// say -v voice -r rate -a audioDevice text

	text = strings.TrimSpace(text)

	if text == "" {
		return nil
	}

	args := []string{}
	if s.voice != "" {
		args = append(args, "-v", s.voice)
	}
	if s.rate != "" {
		args = append(args, "-r", s.rate)
	}
	if s.audioDevice != "" {
		args = append(args, "-a", s.audioDevice)
	}
	args = append(args, text)

	return exec.Command("say", args...).Run()
}

func (s *sayer) Say(text string) error {
	timeout := 15 * time.Second
	ch := make(chan error, 1)

	go func() {
		ch <- s.say(text)
	}()

	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		log.Print("say: timeout")
		return errors.New("say: timeout")
	}
}
