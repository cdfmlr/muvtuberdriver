package chatbot

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"muvtuberdriver/model"
	"net/http"
	"sync/atomic"
	"time"
)

type Chatbot interface {
	Chat(textIn *model.TextIn) (*model.TextOut, error)
}

func TextOutFromChatbot(chatbot Chatbot, textInChan <-chan *model.TextIn, textOutChan chan<- *model.TextOut) {
	for {
		textIn := <-textInChan
		textOut, err := chatbot.Chat(textIn)
		if err != nil {
			log.Printf("chatbot.Chat(%v) failed: %v", textIn, err)
		}
		textOutChan <- textOut
	}
}

type Cooldown struct {
	coolingdown atomic.Bool
	Interval    time.Duration
}

var DefaultCooldownInterval = time.Second * 60

func (c *Cooldown) accessWithCooldown() bool {
	if c.Interval == 0 {
		log.Printf("cooldown interval missing, use default: %v", DefaultCooldownInterval)
		c.Interval = DefaultCooldownInterval
	}

	if c.coolingdown.Load() {
		return false
	}

	c.coolingdown.Store(true)
	go func() {
		time.Sleep(c.Interval)
		c.coolingdown.Store(false)
	}()

	return true
}

// region ChatGPTChatbot

type ChatGPTChatbot struct {
	Server string
	client *http.Client
	Cooldown
}

func (c *ChatGPTChatbot) Chat(textIn *model.TextIn) (*model.TextOut, error) {
	// curl -X POST localhost:9006/ask -d '{"prompt": "你好"}'

	if !c.Cooldown.accessWithCooldown() {
		return nil, errors.New("ChatGPTChatbot is cooling down")
	}

	log.Printf("[ChatGPTChatbot] Chat(%s): %s", textIn.Author, textIn.Content)

	resp, err := c.chat(textIn.Content)
	if err != nil {
		return nil, err
	}

	textOut := model.TextOut{
		Author:   "ChatGPTChatbot",
		Content:  resp,
		Priority: textIn.Priority,
	}

	log.Printf("[ChatGPTChatbot] done: Chat(%s): %s", textOut.Author, textOut.Content)

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

// region PrioritizedChatbot

// PrioritizedChatbot 按照 TextIn 的 Priority 调用 Chatbot。
// 高优先级的 Chatbot 应该是对话质量更高的（例如 ChatGPTChatbot），而低优先级的 Chatbot 用来保底。
// 如果没有对应级别的 Chatbot，会往下滑到更低的级别。
type PrioritizedChatbot struct {
	chatbots map[model.Priority]Chatbot
}

func (p *PrioritizedChatbot) Chat(textIn *model.TextIn) (*model.TextOut, error) {
	if textIn == nil {
		return nil, nil
	}
	log.Printf("[PrioritizedChatbot] Chat(%s): %s", textIn.Author, textIn.Content)

	priority := textIn.Priority

	for i := priority; i >= 0; i-- {
		chatbot, ok := p.chatbots[i]
		if !ok {
			continue
		}

		textOut, err := chatbot.Chat(textIn)
		if err != nil {
			if i == 0 {
				log.Printf("PrioritizedChatbot all Chatbots failed: %v, return nil", err)
				return nil, err
			} else {
				log.Printf("%T.Chat(%v) failed: %v, try next chatbot", chatbot, textIn, err)
				continue
			}
		}
		if textOut != nil {
			log.Printf("[PrioritizedChatbot] Chat(%s): %s => (%s): %s", textIn.Author, textIn.Content, textOut.Author, textOut.Content)
			return textOut, nil
		}
	}

	return nil, nil
}

func NewPrioritizedChatbot(chatbots map[model.Priority]Chatbot) Chatbot {
	return &PrioritizedChatbot{
		chatbots: chatbots,
	}
}

// endregion PrioritizedChatbot
