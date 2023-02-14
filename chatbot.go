package main

import (
	"io"
	"net/http"
)

type Chatbot interface {
	Chat(textIn *TextIn) (*TextOut, error)
}

func TextOutFromChatbot(chatbot Chatbot, textInChan <-chan *TextIn, textOutChan chan<- *TextOut) {
	for {
		textIn := <-textInChan
		textOut, err := chatbot.Chat(textIn)
		if err != nil {
			panic(err)
		}
		textOutChan <- textOut
	}
}

// region MusharingChatbot

type MusharingChatbot struct {
	Server string
	client *http.Client
}

func (m *MusharingChatbot) Chat(textIn *TextIn) (*TextOut, error) {
	// curl '127.0.0.1:5000/chatbot/get_response?chat=是的'

	t := textIn.Content

	resp, err := m.client.Get(m.Server + "/chatbot/get_response?chat=" + t)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	r := TextOut(body)
	return &r, nil
}

func NewMusharingChatbot(server string) Chatbot {
	return &MusharingChatbot{
		Server: server,
		client: &http.Client{},
	}
}

// endregion MusharingChatbot

// region ChatGPTChatbot: TODO

// TODO
type ChatGPTChatbot struct{}

// TODO
func (c *ChatGPTChatbot) Chat(textIn *TextIn) (*TextOut, error) {
	panic("not implemented")
}

// TODO
func NewChatGPTChatbot() Chatbot {
	return &ChatGPTChatbot{}
}

// endregion ChatGPTChatbot: TODO
