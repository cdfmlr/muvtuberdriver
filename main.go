package main

import (
	"fmt"
)

var (
	roomid = 1
)

func main() {
	textInChan := make(chan *TextIn, RecvMsgChanBuf)
	textOutChan := make(chan *TextOut, RecvMsgChanBuf)

	go TextInFromDm(roomid, textInChan)
	go TextInFromHTTP(":9010", "/", textInChan)

	chatbot := NewMusharingChatbot("")
	go TextOutFromChatbot(chatbot, textInChan, textOutChan)

	go func() {
		for {
			textOut := <-textOutChan
			fmt.Println(textOut)
		}
	}()
}
