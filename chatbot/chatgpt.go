package chatbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	api "muvtuberdriver/chatbot/chatbotapi/v2"
	"muvtuberdriver/model"
	"muvtuberdriver/pkg/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const ChatGptRpcTimeout = time.Second * 50

// ChatbotServiceClient = Conn + Client
type ChatbotServiceClient struct {
	conn   *grpc.ClientConn
	client *api.ChatbotServiceClient
}

func (c *ChatbotServiceClient) Close() error {
	if c == nil {
		return nil
	}
	return c.conn.Close()
}

// ChatGPTConfig is the config to a ChatGPT Chatbot server session
type ChatGPTConfig struct {
	Version       int    `json:"version"`
	AccessToken   string `json:"access_token,omitempty"`
	ApiKey        string `json:"api_key,omitempty"`
	InitialPrompt string `json:"initial_prompt,omitempty"`
}

// chatgptSession wraps sessionId to make it Poolable
type chatgptSession struct {
	sessionId sessionId
	failed    int
}

func (s *chatgptSession) Close() error {
	return nil
}

type sessionId = string

type ChatGPTChatbot struct {
	Server string

	// TODO: (stateless): configs should be getting from app config dynamically.
	// configs for creating sessions
	sessionConfigs []ChatGPTConfig
	// next idx to sessions: do not use sessions & nextSessionIdx directly, use nextSession() instead
	nextConfigIdx int

	// gRPC conn pool (clients)
	clientPool pool.Pool[*ChatbotServiceClient]

	// TODO: (stateless): sessionsPool should be in Redis.
	// sessions ready to use
	sessionsPool pool.Pool[*chatgptSession]

	Cooldown
}

func NewChatGPTChatbot(server string, configs []ChatGPTConfig) (Chatbot, error) {
	c := &ChatGPTChatbot{
		Server:         server,
		sessionConfigs: configs,
	}

	// FIXME: hardcode
	c.clientPool = pool.NewPool(10, func() (*ChatbotServiceClient, error) {
		conn, err := grpc.Dial(c.Server, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}

		client := api.NewChatbotServiceClient(conn)

		return &ChatbotServiceClient{
			conn:   conn,
			client: &client,
		}, nil
	})

	// FIXME: hardcode
	c.sessionsPool = pool.NewPool(10, func() (*chatgptSession, error) {
		sessionId, err := c.newSession(c.nextConfig())
		return &chatgptSession{sessionId: sessionId}, err
	})

	return c, nil
}

// newSession do RPC request to create a new ChatGPT session.
// the config argument should be get by c.nextConfig().
func (c *ChatGPTChatbot) newSession(config ChatGPTConfig) (sessionId, error) {
	client, err := c.clientPool.Get()
	if err != nil {
		log.Printf("ChatGPTChatbot.newSession get client error: %v", err)
		return "", err
	}
	defer c.clientPool.Put(client)

	configCopy := config
	configCopy.InitialPrompt = "" // remove redundant field
	configJson, err := json.Marshal(configCopy)
	if err != nil {
		log.Printf("ChatGPTChatbot.newSession marshal config error: %v", err)
		return "", err
	}

	resp, err := (*client.client).NewSession(
		context.Background(),
		&api.NewSessionRequest{
			Config:        string(configJson),
			InitialPrompt: config.InitialPrompt,
		})

	if err != nil {
		log.Printf("ChatGPTChatbot new session error: %v", err)
		return "", err
	}

	log.Printf("ChatGPTChatbot new session: %s", resp.GetSessionId())
	return resp.GetSessionId(), nil
}

// Chat implements the Chatbot interface.
func (c *ChatGPTChatbot) Chat(textIn *model.TextIn) (*model.TextOut, error) {
	// $ grpcurl -d '{"session_id": "b7268187-ab7a-4e2d-9d4a-0161975369bd", "prompt": "hello!!"}' -plaintext localhost:50052 muvtuber.chatbot.chatgpt_chatbot.v1.ChatGPTService.Chat
	// {"response": "Hello! How can I assist you today?"}

	if !c.Cooldown.AccessWithCooldown() {
		return nil, fmt.Errorf("ChatGPTChatbot is cooling down (%v)", c.Cooldown.Interval)
	}

	respContext, err := c.chat(textIn.Content)
	if err != nil {
		return nil, err
	}

	r := model.TextOut{
		Author:   "ChatGPTChatbot",
		Content:  respContext,
		Priority: textIn.Priority,
	}
	return &r, nil
}

// chat gets a session & a client, do PRC request,
// returns the text content of response from Chatbot.
func (c *ChatGPTChatbot) chat(prompt string) (string, error) {
	session, err := c.sessionsPool.Get()
	if err != nil {
		log.Printf("ChatGPTChatbot.chat get session error: %v", err)
		return "", err
	}
	// defer c.sessionsPool.Put(session)

	ctx, cancel := context.WithTimeout(context.Background(), ChatGptRpcTimeout)
	defer cancel()

	client, err := c.clientPool.Get()
	if err != nil {
		log.Printf("ChatGPTChatbot.chat get client error: %v", err)
		c.sessionsPool.Put(session) // this session can be reused.
		return "", err
	}
	defer c.clientPool.Put(client)

	resp, err := (*client.client).Chat(ctx, &api.ChatRequest{
		SessionId: session.sessionId,
		Prompt:    prompt,
	})
	if err != nil {
		session.failed += 1
		log.Printf("ChatGPTChatbot.chat RPC err: %v. Session will be released after successive errors (%v/3).", err, session.failed)
		if session.failed >= 3 { // successive errors, won't reuse this session anymore
			c.sessionsPool.Release(session)
		} else {
			c.sessionsPool.Put(session) // this session can be reused.
		}
		return "", err
	} else {
		session.failed = 0
		c.sessionsPool.Put(session) // use session successfully.
	}

	return resp.GetResponse(), nil
}

func (c *ChatGPTChatbot) nextConfig() ChatGPTConfig {
	if c.nextConfigIdx < 0 || len(c.sessionConfigs) < c.nextConfigIdx {
		panic("ChatGPTChatbot: bad nextConfigIdx or sessionConfigs")
	}
	cfg := c.sessionConfigs[c.nextConfigIdx]
	c.nextConfigIdx = (c.nextConfigIdx + 1) % len(c.sessionConfigs)
	return cfg
}
