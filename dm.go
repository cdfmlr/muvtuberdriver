package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"time"

	"golang.org/x/net/websocket"
)

type blivedmMessage struct {
	Cmd  int `json:"cmd"`
	Data any `json:"data"` // map[string]any or []any
}

// blivedm Cmds
const (
	blivedmCmdHeartbeat         = 0
	blivedmCmdJoinRoom          = 1
	blivedmCmdAddText           = 2
	blivedmCmdAddGift           = 3
	blivedmCmdAddMember         = 4
	blivedmCmdAddSuperChat      = 5
	blivedmCmdDelSuperChat      = 6
	blivedmCmdUpdateTranslation = 7
)

// textMessageData is the data of a text message.
//
// The "data" field of the message is []any.
type textMessageData struct {
	AvatarUrl         string
	Timestamp         int64
	AuthorName        string
	AuthorType        int64
	Content           string
	PrivilegeType     int64
	IsGiftDanmaku     bool
	AuthorLevel       int64
	IsNewbie          bool
	IsMobileVerified  bool
	MedalLevel        int64
	Id                string
	Translation       string
	ContentType       int64
	ContentTypeParams []any // []string
}

// textMessageDataFromArray converts a []any to a TextMessageData.
func textMessageDataFromArray(data []interface{}) (*textMessageData, error) {
	t := &textMessageData{}
	tv := reflect.ValueOf(t).Elem()

	if len(data) != tv.NumField() {
		return nil, fmt.Errorf("data has incorrect number of fields")
	}

	for i := 0; i < tv.NumField(); i++ {
		f := tv.Field(i)

		v := reflect.ValueOf(data[i])

		// special case: convert float64 to int64
		if v.Kind() == reflect.Float64 {
			v = reflect.ValueOf(int64(data[i].(float64)))
		}
		if f.Kind() == reflect.Bool && v.Kind() == reflect.Int64 {
			v = reflect.ValueOf(v.Interface() != 0)
		}

		if f.Kind() != v.Kind() {
			return nil, fmt.Errorf("data has incorrect type of field %d, want %s, got %s", i, f.Kind(), v.Kind())
		}

		f.Set(v)
	}

	return t, nil
}

// heartbeating
const (
	blivedmHeartbeatMessage  = `{"cmd":0,"data":{}}`
	blivedmHeartbeatInterval = 10 * time.Second
)

// Configurable variables (default values for BlivedmClientOptions)
var (
	BlivedmServer   = "ws://localhost:12450/api/chat"
	BlivedmWsOrigin = "http://localhost:12450"
	RecvMsgChanBuf  = 100
)

// blivedmJoinRoomMessage builds a JOIN_ROOM message for blivedm server.
func blivedmJoinRoomMessage(roomid int) string {
	msg := blivedmMessage{
		Cmd: blivedmCmdJoinRoom,
		Data: map[string]any{
			"roomId": roomid,
		},
	}

	j, _ := json.Marshal(msg)
	return string(j)
}

// chatClient handles websocket connection to blivedm server.
// Keeps heartbeating and receives messages (heartbeats may be filtered).
//
// Received messages are sent to recvMsgCh.
//
// Blocks until websocket connection is closed.
func chatClient(ws *websocket.Conn, recvMsgCh chan<- string) {
	heartbeat := time.NewTicker(blivedmHeartbeatInterval)
	defer heartbeat.Stop()

	log.Println("chat client started")

LOOP:
	for {
		select {
		case <-heartbeat.C:
			if err := websocket.Message.Send(ws, blivedmHeartbeatMessage); err != nil {
				break LOOP
			}
		default:
			var msg string
			if err := websocket.Message.Receive(ws, &msg); err != nil {
				break LOOP
			}
			if msg == blivedmHeartbeatMessage { // a quick but unqualified filter
				continue
			}
			recvMsgCh <- msg
		}
	}
}

// newBlivedmClient creates a new websocket connection to blivedm server,
// joins the room and returns a channel for receiving messages.
func newBlivedmClient(roomid int, opts ...BlivedmClientOption) (recvMsgCh <-chan string, err error) {
	o := blivedmClientOptions{
		BlivedmServer:   BlivedmServer,
		BlivedmWsOrigin: BlivedmWsOrigin,
		RecvMsgChanBuf:  RecvMsgChanBuf,
	}
	for _, opt := range opts {
		opt(&o)
	}

	ws, err := websocket.Dial(o.BlivedmServer, "", o.BlivedmWsOrigin)
	if err != nil {
		log.Println("websocket dial error:", err)
		return nil, err
	}

	ch := make(chan string, o.RecvMsgChanBuf)
	if err = websocket.Message.Send(ws, blivedmJoinRoomMessage(roomid)); err != nil {
		return nil, err
	}

	go chatClient(ws, ch)

	return ch, nil
}

type blivedmClientOptions struct {
	BlivedmServer   string
	BlivedmWsOrigin string
	RecvMsgChanBuf  int
}

type BlivedmClientOption func(*blivedmClientOptions)

func WithBlivedmServer(s string) BlivedmClientOption {
	return func(o *blivedmClientOptions) {
		o.BlivedmServer = s
	}
}

func WithBlivedmWsOrigin(s string) BlivedmClientOption {
	return func(o *blivedmClientOptions) {
		o.BlivedmWsOrigin = s
	}
}

func WithRecvMsgChanBuf(n int) BlivedmClientOption {
	return func(o *blivedmClientOptions) {
		o.RecvMsgChanBuf = n
	}
}

// deprecated
//
// blivedmMessageHandler handles a message from blivedm server.
// 分发给 textMessageHandler 等函数处理。
//
// ⚠️ 留着只是作参考，不要用这个函数，功能有错误！！
func blivedmMessageHandler(msg string) error {
	var message blivedmMessage
	if err := json.Unmarshal([]byte(msg), &message); err != nil {
		return err
	}

	switch message.Cmd {
	case blivedmCmdAddText, blivedmCmdAddSuperChat:
		textMessageHandler(&message)
	case blivedmCmdAddGift:
		// TODO
	case blivedmCmdAddMember:
		// TODO
	case blivedmCmdDelSuperChat:
		// TODO
	case blivedmCmdUpdateTranslation:
		// TODO: what the fuck is this?
	}

	return nil
}

func unmarshalMessage(msg string) (*blivedmMessage, error) {
	var message blivedmMessage
	if err := json.Unmarshal([]byte(msg), &message); err != nil {
		return &message, err
	}
	return &message, nil
}

func textMessageHandler(message *blivedmMessage) (*TextIn, error) {
	data, ok := message.Data.([]any)
	if !ok {
		return nil, errors.New("data is not an array")
	}
	tmd, err := textMessageDataFromArray(data)
	if err != nil {
		return nil, err
	}

	// fmt.Println(tmd)

	textIn := &TextIn{
		Author:   tmd.AuthorName,
		Content:  tmd.Content,
		Priority: PriorityLow,
	}

	return textIn, nil
}

// TextInFromDm 从 roomid 的直播间接收弹幕消息，发送到 textIn。
func TextInFromDm(roomid int, textIn chan<- *TextIn, opts ...BlivedmClientOption) (err error) {
	log.Printf("start receiving text from room %d", roomid)
	recvMsgCh, err := newBlivedmClient(roomid, opts...)
	if err != nil {
		return err
	}

	for msg := range recvMsgCh {
		message, err := unmarshalMessage(msg)
		if err != nil {
			fmt.Printf("unmarshalMessage(%s) error: %v\n", msg, err)
			continue
		}

		switch message.Cmd {
		case blivedmCmdAddText, blivedmCmdAddSuperChat:
			t, err := textMessageHandler(message)
			if err != nil {
				fmt.Printf("textMessageHandler(%s) error: %v\n", msg, err)
				continue
			}
			log.Printf("TextInFromDm: %s", t.Content)
			textIn <- t
		}
	}

	return nil
}
