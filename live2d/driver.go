// Package live2d talks to the live2ddriver.
package live2d

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"muvtuberdriver/audio"
	"muvtuberdriver/model"
	"net/http"
	"strings"

	"golang.org/x/exp/slog"
)

type Driver interface {
	TextOutToLive2DDriver(textOut *model.TextOut) error
	Live2dToMotion(motion string) // note: remind the todo documented on live2dDriver.live2dToMotion
	Live2dSpeak(audioContent []byte, expression string, motion string) error
}

type live2dDriver struct {
	Server           string // the zhizuku driver
	MsgForwardServer string // live2dMsgFwd

	client *http.Client
}

func NewDriver(server string, MsgForwardServer string) Driver {
	return &live2dDriver{
		Server:           server,
		MsgForwardServer: MsgForwardServer,
		client:           &http.Client{},
	}
}

func (l *live2dDriver) TextOutToLive2DDriver(textOut *model.TextOut) error {
	// curl -X POST -d '你好' 'localhost:9011'
	if textOut == nil {
		return errors.New("textOut is nil")
	}

	resp, err := l.client.Post(l.Server, "text/plain",
		bytes.NewReader([]byte(textOut.Content)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// i don't care about the response body

	return nil
}

// Live2dToMotion sends motion command to live2d model
//
// TODO: 在 driver 层支持基本动作的调用，不要想现在这样直接拿 wsforwarder 传。
// require: new driver (gRPC) api.
//
//	curl -X POST localhost:9002/live2d -H 'Content-Type: application/json' -d '{"motion": "idle"}'
func (l *live2dDriver) Live2dToMotion(motion string) {
	client := &http.Client{}
	var data = strings.NewReader(fmt.Sprintf(`{"motion": "%s"}`, motion))
	req, err := http.NewRequest("POST", l.MsgForwardServer, data)
	if err != nil {
		slog.Warn("[live2dToMotion] http.NewRequest failed. err=%v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("[live2dToMotion] http client do request failed. err=%v", err)
	}
	defer resp.Body.Close()
}

// Live2dSpeak sends audio content to live2ddriver.
//
//	curl -X POST localhost:9002/live2d -H 'Content-Type: application/json' \
//	     -d '{"speak": { "audio": "src", "expression": "id", "motion": "group" }}'
//
// In which "audio" is the audio source: an url to audio file (wav or mp3) or
// a base64 encoded data (data:audio/wav;base64,xxxx)
func (l *live2dDriver) Live2dSpeak(audioContent []byte, expression string, motion string) error {
	client := &http.Client{}

	//var data = strings.NewReader(
	//	fmt.Sprintf(`{"speak": {"audio": "%s", "expression": "%s", "motion": "%s"}}`,
	//		audio.Base64EncodeAudio("wav", audioContent), expression, motion))
	data := constructSpeakData(audioContent, expression, motion)

	req, err := http.NewRequest("POST", l.MsgForwardServer, data)
	if err != nil {
		slog.Warn("[live2dSpeak] http.NewRequest failed. err=%v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("[live2dSpeak] http client do request failed. err=%v", err)
	}
	defer resp.Body.Close()

	return nil
}

func constructSpeakData(audioContent []byte, expression string, motion string) io.Reader {
	type speak struct {
		Audio      string `json:"audio,omitempty"`
		Expression string `json:"expression,omitempty"`
		Motion     string `json:"motion,omitempty"`
	}

	type speakData struct {
		Speak speak `json:"speak,omitempty"`
	}

	var data = speakData{
		Speak: speak{
			Audio:      audio.Base64EncodeAudio("audio/wav", audioContent),
			Expression: expression,
			Motion:     motion,
		}}

	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(data)
	if err != nil {
		slog.Error("[live2dSpeak] json encode failed. err=%v", err)
	}

	return &buf
}
