package main

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"muvtuberdriver/pkg/wsforwarder"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/slog"
	"golang.org/x/net/websocket"
)

const CleanReportAfter = 5 * time.Minute

type AudioController interface {
	AudioToTrack(format string, audio []byte) *Track

	PlayBgm(track *Track) error
	PlayFx(track *Track) error
	PlaySing(track *Track) error
	PlayVocal(track *Track) error

	WsHandler() http.Handler

	// Wait for the audioview report the status of the track playing command.
	//
	// if there are multiple audioview, ANY one of them reports the status
	// will trigger the wait to return.
	//
	// Example:
	// 	// wait for the audioview starting to play the track
	// 	_ := c.Wait(ctx, ReportStart(track.ID))
	// 	// wait for the audioview finishing playing the track
	// 	_ := c.Wait(ctx, ReportEnd(track.ID))
	Wait(ctx context.Context, report *Report) error
}

// this file implement the audio controller for the audioview,
// that is, a websocket server that sends audio to the audioview.

type audioController struct {
	forwarder wsforwarder.Forwarder

	reports         sync.Map // map[string]time.Time: "report.String()" -> (recv time)
	cleaningReports sync.Mutex
}

func NewAudioController() AudioController {
	return &audioController{
		forwarder: wsforwarder.NewMessageForwarder(),
	}
}

func (c *audioController) WsHandler() http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		// receive
		go c.recv(conn)
		// send
		c.forwarder.ForwardMessageTo(conn)
	})
}

func (c *audioController) PlayVocal(track *Track) error {
	return c.sendPlayCmd(CmdPlayVocal, track)
}

func (c *audioController) PlaySing(track *Track) error {
	return c.sendPlayCmd(CmdPlaySing, track)
}

func (c *audioController) PlayFx(track *Track) error {
	return c.sendPlayCmd(CmdPlayFx, track)
}

func (c *audioController) PlayBgm(track *Track) error {
	return c.sendPlayCmd(CmdPlayBgm, track)
}

// AudioToTrack converts the audio to a Track object.
// The audio content is encoded in base64 and put into the src field
// in data url format:
//
//	"data:[<mediatype>][;base64],<data>"
//
// the ID field will be set to a hash of the audio content.
//
// TODO: sayer += ID & let audioController reuse it to identify the track
func (c *audioController) AudioToTrack(format string, audio []byte) *Track {
	var dataurl strings.Builder
	dataurl.WriteString("data:")
	dataurl.WriteString(format)
	dataurl.WriteString(";base64,")

	base64Content := base64.StdEncoding.EncodeToString(audio)
	dataurl.WriteString(base64Content)

	audioHash := md5.Sum(audio)

	return &Track{
		ID:     fmt.Sprintf("%x", audioHash),
		Src:    dataurl.String(),
		Format: format,
	}
}

func (c *audioController) sendPlayCmd(cmd string, track *Track) error {
	// construct the command
	command := AudioMessage{
		Cmd:  cmd,
		Data: track,
	}

	j, err := json.Marshal(command)
	if err != nil {
		return err
	}

	// send the command
	c.forwarder.SendMessage(j)

	return nil
}

func (c *audioController) Wait(ctx context.Context, report *Report) error {
	waitingReport := report.String()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// check if the report is received
			if _, ok := c.reports.LoadAndDelete(waitingReport); ok {
				return nil
			}
		}
	}
}

// recv receives the messages (keepAlive | report) from the audioview.
// Blocks until the connection is closed.
func (c *audioController) recv(conn *websocket.Conn) {
	for {
		var msg AudioMessage
		err := websocket.JSON.Receive(conn, &msg)
		if err != nil {
			slog.Warn("[audioController] recv: receive msg failed", "err", err)
			return
		}

		switch msg.Cmd {
		case "keepAlive":
			// do nothing
		case "report":
			c.handleReport(&msg)
		default:
			slog.Warn("[audioController] recv: unknown cmd", "cmd", msg.Cmd)
		}

		go c.cleanUnusedReports()
	}
}

func (c *audioController) handleReport(msg *AudioMessage) {
	// save the report
	if msg.Data == nil {
		slog.Warn("[audioController] recv report failed: data is nil")
		return
	}
	// map[string]any -> Report
	// TODO: performance
	j, err := json.Marshal(msg.Data)
	if err != nil {
		slog.Error("[audioController] recv report: failed to convert recved msg Data -> Report", "err", err)
		return
	}
	var report Report
	json.Unmarshal(j, &report)

	if report.ID == "" {
		slog.Warn("[audioController] recv report failed: ID is empty")
		return
	}
	if report.Status != StatusStart && report.Status != StatusEnd {
		slog.Error("report status is not start or end", "status", report.Status)
		return
	}
	slog.Info("[audioController] recv report successfully", "report", report)
	c.reports.Store(report.String(), time.Now())
}

// cleanUnusedReports cleans the reports that are not used for 5 minutes.
//
// cleanUnusedReports may block for 5 minutes.
// call it in a goroutine.
func (c *audioController) cleanUnusedReports() {
	if ok := c.cleaningReports.TryLock(); !ok {
		return
	}
	defer c.cleaningReports.Unlock()

	// clear the report after 5 minutes
	//
	// I failed to understand the doc of sync.Map:
	//  "Range does not necessarily correspond to any consistent snapshot of the Map's contents ..."
	// wtf... so elements here may be deleted twice?
	//
	// Well, gave up, just lock it.
	c.reports.Range(func(key, value interface{}) bool {
		t, ok := value.(time.Time)
		if !ok {
			slog.Error("report value is not a time.Time")
			return true
		}
		if time.Since(t) > CleanReportAfter {
			c.reports.Delete(key)
		}
		return true
	})

	// we don't need to clean the reports every time.
	// just sleep for 5 minutes with the lock held.
	// new cleanUnusedReports() calls will return immediately
	// until the lock is released.
	time.Sleep(CleanReportAfter)
}

// AudioMessage is the command msg sent to the audioview.
type AudioMessage struct {
	Cmd  string `json:"cmd"`
	Data any    `json:"data"` // Track | Report
}

// Track is a audio playing task.
// It should be named AudioTask, but I'm too lazy to change it.
type Track struct {
	ID       string  `json:"id"` // used to identify the track & report progress (start, end, etc.)
	Src      string  `json:"src"`
	Format   string  `json:"format,omitempty"`
	Volume   float64 `json:"volume,omitempty"`
	PlayMode string  `json:"playMode,omitempty"` // PlayMode should be named PlayAt, it's indicating when to play the track
}

// Report is the report msg sent from the audioview.
type Report struct {
	ID     string          `json:"id"`     // the ID of the track
	Status AudioPlayStatus `json:"status"` // the status of the track: start | end
}

func ReportStart(id string) *Report {
	return &Report{
		ID:     id,
		Status: StatusStart,
	}
}

func ReportEnd(id string) *Report {
	return &Report{
		ID:     id,
		Status: StatusEnd,
	}
}

func (r *Report) String() string {
	return fmt.Sprintf("Report(%s: %s)", r.ID, r.Status)
}

// PlayModes
const (
	PlayAtNext      = "next"
	PlayAtNow       = "now"
	PlayAtResetNext = "resetNext"
	PlayAtResetNow  = "resetNow"
)

// cmds
const (
	CmdPlayBgm   = "playBgm"
	CmdPlayFx    = "playFx"
	CmdPlaySing  = "playSing"
	CmdPlayVocal = "playVocal"
)

// AudioPlayStatus: StatusStart | StatusEnd
type AudioPlayStatus string

// status from report
// XXX: 有没有必要引入 err 状态呢？
const (
	StatusStart AudioPlayStatus = "start"
	StatusEnd   AudioPlayStatus = "end"
)
