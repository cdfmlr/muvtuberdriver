package main

import (
	"bytes"
	"errors"
	"net/http"
)

type Live2DDriver interface {
	TextOutToLive2DDriver(textOut *TextOut) error
}

type live2dDriver struct {
	Server string
	client *http.Client
}

func NewLive2DDriver(server string) Live2DDriver {
	return &live2dDriver{
		Server: server,
		client: &http.Client{},
	}
}

func (l *live2dDriver) TextOutToLive2DDriver(textOut *TextOut) error {
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
