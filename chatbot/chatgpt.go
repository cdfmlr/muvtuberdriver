package chatbot

import (
	"encoding/json"
	"errors"
	"fmt"
	"muvtuberdriver/model"
	"time"
)

// ChatGPTConfig is the config to a ChatGPT Chatbot server session.
//
// ChatGPTConfig implements the ChatbotConfig interface.
// (Marvelous editor, my foot! VS Code can't even tip me off on which interface a struct implements.)
type ChatGPTConfig struct {
	// ⬇️ Config
	Version     int    `json:"version"`
	AccessToken string `json:"access_token,omitempty"`
	ApiKey      string `json:"api_key,omitempty"`

	// ⬇️ InitPrompt
	InitialPrompt string `json:"initial_prompt,omitempty"`
}

func (c ChatGPTConfig) Config() string {
	configCopy := c
	configCopy.InitialPrompt = ""

	b, _ := json.Marshal(configCopy)
	return string(b)
}

func (c ChatGPTConfig) InitPrompt() string {
	return c.InitialPrompt
}

// chatGPTChatbot is a Chatbot that talks to a ChatbotService server
// that uses the ChatGPT API.
type chatGPTChatbot struct {
	*SessionClientsPool
	Cooldown
}

func NewChatGPTChatbot(addr string, cooldown time.Duration, configs ...ChatGPTConfig) (Chatbot, error) {
	scp, err := NewSessionClientsPool(addr, CastToChatbotConfig(configs)...)
	if err != nil {
		return nil, err
	}

	scp.Name = "ChatGPTChatbot"
	scp.Verbose = true

	return &chatGPTChatbot{
		SessionClientsPool: scp,
		Cooldown:           Cooldown{Interval: cooldown},
	}, nil
}

func (c *chatGPTChatbot) Chat(textIn *model.TextIn) (*model.TextOut, error) {
	if !c.TryCooldown() {
		return nil, fmt.Errorf("%w: %v / %v", ErrCooldown,
			c.CooldownLeftTime(), c.Interval)
	}

	return c.SessionClientsPool.Chat(textIn)
}

var ErrCooldown = errors.New("Chatbot is cooling down")
