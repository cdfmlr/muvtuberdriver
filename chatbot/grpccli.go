package chatbot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	chatbotv2 "muvtuberdriver/chatbot/proto"
	"muvtuberdriver/model"
	"muvtuberdriver/pkg/ellipsis"
	"muvtuberdriver/pkg/pool"

	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var MaxConsecutiveFailures = 3

const DefaultRPCTimeout = time.Second * 60
const DefaultClientPoolSize = 10

// Client is a gRPC client for the ChatbotService
type Client struct {
	conn   *grpc.ClientConn
	client chatbotv2.ChatbotServiceClient

	RPCTimeout time.Duration

	pool.Poolable // 其实这种写法不对，好像，嵌入一个 interface 是用来 wrap interface 的，不是用来提示实现了哪些接口的。。。see sort.Reverse
}

// NewClient creates a new Client
func NewClient(addr string) (*Client, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:       conn,
		client:     chatbotv2.NewChatbotServiceClient(conn),
		RPCTimeout: DefaultRPCTimeout,
	}, nil
}

// ChatbotConfig is the config to a chatbotv2.ChatbotServiceServer
//
// TODO: 把 InitialPrompt 并入 Config。为啥单独列一项啊，纯制造麻烦。。
type ChatbotConfig interface {
	Config() string
	InitPrompt() string
}

// NoChatbotConfig is a ChatbotConfig that is used when the chatbot server
// does not require any config.
// It's Config() method returns an empty string "".
type NoChatbotConfig struct{}

func (c NoChatbotConfig) Config() string {
	return ""
}

func (c NoChatbotConfig) InitPrompt() string {
	return ""
}

// CastToChatbotConfig is a helper function to cast a slice of Chatbot
// implementations to a slice of ChatbotConfig.
//
//	[]struct -> []interface
//
// 😭 It's O(n).
func CastToChatbotConfig[T ChatbotConfig](configs []T) []ChatbotConfig {
	cs := make([]ChatbotConfig, len(configs))
	for i, c := range configs {
		cs[i] = c
	}
	return cs
}

func (c *Client) NewSession(config ChatbotConfig) (*Session, error) {
	resp, err := c.client.NewSession(
		context.Background(),
		&chatbotv2.NewSessionRequest{
			Config:        config.Config(),
			InitialPrompt: config.InitPrompt(),
		},
	)

	if err != nil {
		return nil, err
	}

	session := &Session{
		client:    c,
		SessionID: resp.GetSessionId(),
	}

	return session, nil
}

// Close closes the ChatbotClient
func (c *Client) Close() error {
	return c.conn.Close()
}

type SessionId = string

// Session is a session of a Client.
//
// Session can be created by Client.NewSession().
// It implements the Chatbot interface: You can call Chat() to do the RPC.
type Session struct {
	client             *Client
	SessionID          SessionId
	successiveFailures int // successive failures of Chat. ChatbotSession.Chat mantains this value, and should not be modified by other code.

	AuthorName string // the author name of the chatbot: "AnonymousChatbot" by default.

	pool.Poolable
}

// Chat implements the Chatbot interface.
//
// It calls the chat to do the RPC, handles the error (successive failures),
// and construct the response (TextOut).
func (s *Session) Chat(textIn *model.TextIn) (*model.TextOut, error) {
	respContent, err := s.chat(textIn.Content)

	if err != nil {
		s.successiveFailures++
		return nil, err
	}
	s.successiveFailures = 0

	if s.AuthorName == "" {
		s.AuthorName = "AnonymousChatbot"
	}

	resp := &model.TextOut{
		Author:   s.AuthorName,
		Content:  respContent,
		Priority: textIn.Priority,
	}

	return resp, nil
}

// chat do PRC (with timeout). Returns the text content of response from Chatbot.
func (s *Session) chat(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.client.RPCTimeout)
	defer cancel()

	resp, err := s.client.client.Chat(ctx, &chatbotv2.ChatRequest{
		SessionId: s.SessionID,
		Prompt:    prompt,
	})

	if err != nil {
		return "", err
	}

	return resp.GetResponse(), nil
}

// Close the ChatbotSession by calling the DeleteSession RPC.
func (s *Session) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.client.RPCTimeout)
	defer cancel()
	_, err := s.client.client.DeleteSession(
		ctx,
		&chatbotv2.DeleteSessionRequest{
			SessionId: s.SessionID,
		},
	)
	return err
}

// SuccessiveFailures returns the successive failures of Chat RPC.
func (s *Session) SuccessiveFailures() int {
	return s.successiveFailures
}

// SessionClient = a Client + a Session over the Client.
//
// A ChatbotClient is just a gRPC client (conn).
// And a Session is a session over a Client (an
// implementation to the Chatbot interface).
// We can create multiple ChatbotSessions over a ChatbotClient:
//
//	client, _ := chatbot.NewClient(addr)
//
//	session1, _ := client.NewSession(config{"you are hatsune"})
//	session2, _ := client.NewSession(config{"you are miku"})
//
//	textOut1, _ := session1.Chat(textIn)
//	textOut2, _ := session2.Chat(textIn)
//
// SessionClient, however, is a lazy initialized Client
// bound with a lazy created Session.
// We can create a SessionClient, and use it as a Chatbot directly:
//
//	cs, _ := chatbot.NewSessionClient(addr, config{"you are hatsune miku"})
//	textOut, _ := cs.Chat(textIn)
//
// It's lazy: both Client and Session are created when the first
// Chat is called.
type SessionClient struct {
	addr   string
	config ChatbotConfig

	client  *Client
	session *Session

	Name string // the name of the chatbot: for Session.AuthorName

	Quiet bool // if true, it will not log.
}

func NewSessionClient(addr string, config ChatbotConfig) (*SessionClient, error) {
	if addr == "" {
		return nil, errors.New("addr is empty")
	}
	if config == nil {
		config = NoChatbotConfig{}
	}

	return &SessionClient{
		addr:   addr,
		config: config,
	}, nil
}

// Chat implements the Chatbot interface.
// It do the RPC, and returns the response.
//
// if session is not created, it creates a new one.
// if client is not created, it creates a new one.
func (c *SessionClient) Chat(textIn *model.TextIn) (*model.TextOut, error) {
	if textIn == nil {
		return nil, errors.New("textIn is nil")
	}

	slog.Info("[chatbot] SessionClient Chat: got textIn:",
		"chatbotName", c.Name, "textin", ellipsis.Centering(textIn.Content, 11))

	if err := c.initClientIfNil(); err != nil {
		return nil, err
	}
	if err := c.initSessionIfNil(); err != nil {
		return nil, err
	}

	return c.chat(textIn)
}

func (c *SessionClient) initClientIfNil() error {
	if c.client != nil {
		return nil
	}

	client, err := NewClient(c.addr)
	if err != nil {
		err = fmt.Errorf("NewClient(addr=%v) failed: %w", c.addr, err)
		return err
	}

	c.client = client
	if !c.Quiet {
		slog.Info("[chatbot] SessionClient Chat: NewClient created.",
			"chatbot", c.Name, "addr", c.addr)
	}

	return nil
}

func (c *SessionClient) initSessionIfNil() error {
	if c.session != nil {
		return nil
	}

	session, err := c.client.NewSession(c.config)
	if err != nil {
		err = fmt.Errorf("NewSession(addr=%v) failed: %w", c.addr, err)
		return err
	}

	session.AuthorName = c.Name

	c.session = session

	if !c.Quiet {
		slog.Info("[chatbot] SessionClient Chat: NewSession created.",
			"chatbot", c.Name, "addr", c.addr, "sessionID", session.SessionID)
	}

	return nil
}

// chat calls session.Chat and logs.
func (c *SessionClient) chat(textIn *model.TextIn) (*model.TextOut, error) {
	textOut, err := c.session.Chat(textIn)

	if err != nil {
		err = fmt.Errorf("Chat(addr=%v) failed: %w", c.addr, err)
		return nil, err
	}
	if textOut == nil {
		return nil, errors.New("textOut is nil")
	}

	if !c.Quiet {
		slog.Info("[chatbot] SessionClient Chat success.",
			"chatbot", c.Name,
			// "addr", c.addr,
			"sessionID", ellipsis.Ending(c.session.SessionID, 10),
			"textin", ellipsis.Centering(textIn.Content, 11),
			"textout", ellipsis.Centering(textOut.Content, 11),
		)
	}

	return textOut, nil
}

// Close closes the SessionClient.
// It closes the Client and Session.
func (c *SessionClient) Close() error {
	var errS, errC error
	if c.session != nil {
		errS = c.session.Close()
	}
	if c.client != nil {
		errC = c.client.Close()
	}
	return errors.Join(errS, errC)
}

func (c *SessionClient) SuccessiveFailures() int {
	if c.session == nil {
		return 0
	}
	return c.session.SuccessiveFailures()
}

// SessionClientsPool is a pool of SessionClient.
//
// SessionClientsPool implements the Chatbot interface.
// So it can be used as a Chatbot:
//
//	ccsp, _ := NewSessionClientsPool(addr, config{"you are hatsune miku"})
//	textOut, _ := ccsp.Chat(textIn).
type SessionClientsPool struct {
	// pool holds the ChatbotClientWithSessions.
	pool pool.Pool[*SessionClient]

	configs       []ChatbotConfig
	nextConfigIdx int
	configsMu     sync.Mutex

	// ⬇️ 这些都是可选的：我觉得这层可以打日志了，别麻烦调用者。但又觉得这样不太好，耦合功能了。

	Verbose bool   // if true, it will log the errors.
	Name    string // for logging.
}

// NewSessionClientsPool creates a new SessionClientsPool
// that connects to the given addr.
//
// Multiple configs can be given, and they will be used in a round-robin fashion.
func NewSessionClientsPool(addr string, configs ...ChatbotConfig) (*SessionClientsPool, error) {
	if configs == nil {
		// configs = []ChatbotConfig{NoChatbotConfig{}}
		return nil, errors.New("configs is nil")
	}
	ccsp := &SessionClientsPool{
		configs: configs,
	}

	ccsp.pool = pool.NewPool(
		DefaultClientPoolSize,
		func() (*SessionClient, error) {
			return NewSessionClient(addr, ccsp.nextConfig())
		},
	)

	return ccsp, nil
}

// Chat implements the Chatbot interface.
//
// if SessionClientsPool.Quiet is false, it will log the errors.
//
// 我这里 Chat 做日志，chat 做实际工作 + errors wrap 是想做个例子，
// 可以这样来包装错误，**推迟**或**集中**日志的打印工作，避免代码里到处是 log。
// 只是一种实验。
func (p *SessionClientsPool) Chat(textIn *model.TextIn) (*model.TextOut, error) {
	textOut, err := p.chat(textIn)
	if err != nil && p.Verbose {
		if p.Name == "" {
			p.Name = "SessionClientsPool"
		}
		suffix := fmt.Sprintf("[chatbot] %s ", p.Name)
		switch {
		case errors.Is(err, ErrGetSessionClient):
			slog.Error(suffix + err.Error())
		case errors.Is(err, ErrChatFailed):
			slog.Warn(suffix + err.Error())
		case errors.Is(err, ErrChatMaxFailures):
			slog.Error(suffix + err.Error())
		default:
			slog.Error(err.Error())
		}
	}
	return textOut, err
}

// chat gets a SessionClient from the pool, and calls Chat on it.
func (p *SessionClientsPool) chat(textIn *model.TextIn) (*model.TextOut, error) {
	// get a session from the pool
	session, err := p.pool.Get()
	if err != nil {
		err = fmt.Errorf("%w: err=%w", ErrGetSessionClient, err)
		return nil, err
	}
	if session == nil {
		panic("get a nil session from the pool")
	}
	if session.Name == "" {
		session.Name = p.Name
	}

	// call Chat on the client session
	textOut, err := session.Chat(textIn)

	// put the session back into the pool or release it
	// and return the result
	switch {
	case err == nil:
		// success: put it back into the pool
		p.pool.Put(session)

		return textOut, nil

	case err != nil && (session.SuccessiveFailures() >= MaxConsecutiveFailures):
		// too many failures: won't reuse this session anymore: release it from the pool (close it)
		p.pool.Release(session)

		// do not log session: it cantains the CONFIG which may leak OpenAI API key.
		err = fmt.Errorf("%w: serAddr=%v failures=%v/%v err=%w", ErrChatMaxFailures,
			session.addr, session.SuccessiveFailures(), MaxConsecutiveFailures, err)

		return nil, err

	case err != nil:
		// put it back into the pool: try to reuse it
		p.pool.Put(session)

		err = fmt.Errorf("%w: serAddr=%v failures=%v/%v err=%w", ErrChatFailed,
			session.addr, session.SuccessiveFailures(), MaxConsecutiveFailures, err)

		return nil, err
	}

	// panic("unreachable")
	err = fmt.Errorf("%w: textIn=%v serAddr=%v textOut=%v err=%w",
		ErrUnexpectedCase, textIn, session.addr, textOut, err)

	return textOut, err
}

func (p *SessionClientsPool) nextConfig() ChatbotConfig {
	p.configsMu.Lock()
	defer p.configsMu.Unlock()

	if p.nextConfigIdx < 0 || len(p.configs) < p.nextConfigIdx {
		panic(fmt.Sprintf(
			"ChatbotSessionClientsPool: UNEXPECTED nextConfigIdx or configs: nextConfigIdx=%d, configs=%d",
			p.nextConfigIdx, p.configs))
	}

	cfg := p.configs[p.nextConfigIdx]
	p.nextConfigIdx = (p.nextConfigIdx + 1) % len(p.configs)
	return cfg
}

var ErrGetSessionClient = errors.New("failed to get a SessionClient from the pool")
var ErrChatFailed = errors.New("Chat() failed. The SessionClient will be released if successive failures")
var ErrChatMaxFailures = errors.New("Chat() failed. The SessionClient was removed from the pool due to too many consecutive failures")
var ErrUnexpectedCase = errors.New("PROGRAM REACHED A UNREACHABLE CASE")
