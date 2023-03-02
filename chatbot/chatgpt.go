package chatbot

import (
	"context"
	"errors"
	"log"
	api "muvtuberdriver/chatbot/chatgpt_chatbot/v1"
	"muvtuberdriver/model"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const ChatGptRpcTimeout = time.Second * 30

type sessionId = string

type ChatGPTChatbot struct {
	Server string
	client api.ChatGPTServiceClient

	// sessions: do not use sessions & nextSessionIdx directly, use nextSession() instead
	sessions []sessionId
	// next idx to sessions: do not use sessions & nextSessionIdx directly, use nextSession() instead
	nextSessionIdx int

	Cooldown

	// TODO: Do not store Contexts inside a struct type.
	// 这个设计错误了，不应该始终保持一个 conn，用连接池啊，
	// 但这里懒得写，也不想引入额外包，每次 call 时 lazy dail 性能感觉太爆炸，
	// 所以就这样吧。
	gRPCContext context.Context
	Cancel      context.CancelFunc // cancel current gRPC call and close connection.
}

func NewChatGPTChatbot(server string, accessTokens []string, prompt string) (Chatbot, error) {
	// conn, err := grpc.Dial(server, grpc.WithInsecure())
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	ctx.Done()

	conn, err := grpc.DialContext(ctx, server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		cancel()
		return nil, err
	}

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	apiClient := api.NewChatGPTServiceClient(conn)

	c := &ChatGPTChatbot{
		Server: server,
		client: apiClient,

		gRPCContext: ctx,
		Cancel:      cancel,
	}

	// init sessions
	c.sessions = make([]sessionId, len(accessTokens))
	for i, accessToken := range accessTokens {
		session, err := c.newSession(ctx, accessToken, prompt)
		if err != nil {
			return nil, err
		}
		c.sessions[i] = session
	}

	return c, nil
}

func (c *ChatGPTChatbot) newSession(ctx context.Context, accessToken string, prompt string) (sessionId, error) {
	resp, err := c.client.NewSession(ctx, &api.NewSessionRequest{
		AccessToken:   accessToken,
		InitialPrompt: prompt,
	})
	if err != nil {
		log.Printf("ChatGPTChatbot new session error: %v", err)
		return "", err
	}
	log.Printf("ChatGPTChatbot new session: %s", resp.GetSessionId())
	return resp.GetSessionId(), nil
}

func (c *ChatGPTChatbot) Chat(textIn *model.TextIn) (*model.TextOut, error) {
	// $ grpcurl -d '{"session_id": "b7268187-ab7a-4e2d-9d4a-0161975369bd", "prompt": "hello!!"}' -plaintext localhost:50052 muvtuber.chatbot.chatgpt_chatbot.v1.ChatGPTService.Chat
	// {"response": "Hello! How can I assist you today?"}

	if !c.Cooldown.AccessWithCooldown() {
		return nil, errors.New("ChatGPTChatbot is cooling down")
	}

	session := c.nextSession()

	ctx, cancel := context.WithTimeout(c.gRPCContext, ChatGptRpcTimeout)
	defer cancel()

	resp, err := c.client.Chat(ctx, &api.ChatRequest{
		SessionId: session,
		Prompt:    textIn.Content,
	})
	if err != nil {
		return nil, err
	}

	r := model.TextOut{
		Author:   "ChatGPTChatbot",
		Content:  resp.GetResponse(),
		Priority: textIn.Priority,
	}
	return &r, nil
}

func (c *ChatGPTChatbot) nextSession() sessionId {
	session := c.sessions[c.nextSessionIdx]
	c.nextSessionIdx = (c.nextSessionIdx + 1) % len(c.sessions)
	return session
}
