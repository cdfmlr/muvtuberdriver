package audio

import "fmt"

// Message is the command msg sent to the audioview.
type Message struct {
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
	ID     string     `json:"id"`     // the ID of the track
	Status PlayStatus `json:"status"` // the status of the track: start | end
}

func ReportStart(id string) *Report {
	return &Report{
		ID:     id,
		Status: PlayStatusStart,
	}
}

func ReportEnd(id string) *Report {
	return &Report{
		ID:     id,
		Status: PlayStatusEnd,
	}
}

func (r *Report) String() string {
	return fmt.Sprintf("Report(%s: %s)", r.ID, r.Status)
}

type PlayAt string

// PlayModes
const (
	PlayAtNext      PlayAt = "next"
	PlayAtNow       PlayAt = "now"
	PlayAtResetNext PlayAt = "resetNext"
	PlayAtResetNow  PlayAt = "resetNow"
)

// cmds
const (
	CmdPlayBgm   = "playBgm"
	CmdPlayFx    = "playFx"
	CmdPlaySing  = "playSing"
	CmdPlayVocal = "playVocal"

	CmdReset = "reset"
)

// PlayStatus: StatusStart | StatusEnd
type PlayStatus string

// status from report
const (
	PlayStatusStart PlayStatus = "start"
	PlayStatusEnd   PlayStatus = "end"
	PlayStatusErr   PlayStatus = "err"
)
