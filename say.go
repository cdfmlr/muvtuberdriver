package main

import "os/exec"

type Sayer interface {
	Say(text string) error
}

// sayer is a caller of the SAY(1) command on macOS.
type sayer struct {
	voice       string // Specify the voice to be used.
	rate        string // Speech rate to be used, in words per minute.
	audioDevice string // Specify, by ID or name prefix, an audio device to be used to play the audio
}

func (s *sayer) Say(text string) error {
	// say -v voice -r rate -a audioDevice text
	return exec.Command("say",
		"-v", s.voice, "-r", s.rate, "-a", s.audioDevice,
		text,
	).Run()
}
