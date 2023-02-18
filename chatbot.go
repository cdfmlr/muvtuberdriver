package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
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
			log.Printf("chatbot.Chat(%v) failed: %v", textIn, err)
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

	var respBody struct {
		ChatbotResp string `json:"chatbot_resp"`
	}
	err = json.Unmarshal(body, &respBody)
	if err != nil {
		return nil, err
	}

	r := TextOut(respBody.ChatbotResp)
	return &r, nil
}

func NewMusharingChatbot(server string) Chatbot {
	return &MusharingChatbot{
		Server: server,
		client: &http.Client{},
	}
}

// endregion MusharingChatbot

// region ChatGPTChatbot

// TODO
type ChatGPTChatbot struct {
	Server string
	client *http.Client
}

func (c *ChatGPTChatbot) Chat(textIn *TextIn) (*TextOut, error) {
	// curl -X POST localhost:9006/ask -d '{"prompt": "你好"}'

	log.Printf("[ChatGPTChatbot] Chat(%s): %s", textIn.Author, textIn.Content)

	resp, err := c.chat(textIn.Content)
	if err != nil {
		return nil, err
	}

	textOut := TextOut(resp)

	return &textOut, nil
}

func (c *ChatGPTChatbot) chat(textIn string) (string, error) {
	// curl -X POST localhost:9006/ask -d '{"prompt": "你好"}'

	reqBody := map[string]string{
		"prompt": textIn,
	}

	reqBodyJson, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Post(c.Server+"/ask", "application/json",
		bytes.NewReader(reqBodyJson))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func (c *ChatGPTChatbot) renew(accessToken string) error {
	// curl -X POST localhost:9006/renew -d '{"access_token": "eyJhb***99A"}'

	log.Printf("[ChatGPTChatbot] renewing access token")

	reqBody := map[string]string{
		"access_token": accessToken,
	}
	reqBodyJson, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	resp, err := c.client.Post(c.Server+"/renew", "application/json",
		bytes.NewReader(reqBodyJson))
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("renew failed")
	}

	return nil
}

func NewChatGPTChatbot(server string, accessToken string, prompt string) Chatbot {
	c := &ChatGPTChatbot{
		Server: server,
		client: &http.Client{},
	}

	if err := c.renew(accessToken); err != nil {
		log.Printf("[ChatGPTChatbot] renew failed: %v", err)
	}
	if resp, err := c.chat(prompt); err != nil {
		log.Printf("[ChatGPTChatbot] chat failed: %v", err)
	} else {
		log.Printf("[ChatGPTChatbot] chat(prompt) = %s", resp)
	}

	return c
}

// endregion ChatGPTChatbot
