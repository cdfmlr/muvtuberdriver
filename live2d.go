package main

import (
	"bytes"
	"errors"
	"fmt"
	"muvtuberdriver/model"
	"net/http"
	"strings"

	"golang.org/x/exp/slog"
)

type Live2DDriver interface {
	TextOutToLive2DDriver(textOut *model.TextOut) error
	live2dToMotion(motion string) // unexport this to remind the todo documented on live2dDriver.live2dToMotion
}

type live2dDriver struct {
	Server           string // the zhizuku driver
	MsgForwardServer string // live2dMsgFwd

	client *http.Client
}

func NewLive2DDriver(server string, MsgForwardServer string) Live2DDriver {
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

// live2dToMotion sends motion command to live2d model
//
// TODO: 在 driver 层支持基本动作的调用，不要想现在这样直接拿 wsforwarder 传。
// require: new driver (gRPC) api.
//
//	curl -X POST localhost:9002/live2d -H 'Content-Type: application/json' -d '{"motion": "idle"}'
func (l *live2dDriver) live2dToMotion(motion string) {
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
