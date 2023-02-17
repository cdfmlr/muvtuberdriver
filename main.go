package main

import (
	"fmt"
)

var (
	roomid           = 1
	live2dDriverAddr = "http://localhost:9004/driver"
)

func main() {
	textInChan := make(chan *TextIn, RecvMsgChanBuf)
	textOutChan := make(chan *TextOut, RecvMsgChanBuf)

	// (dm) & (http) -> in
	go TextInFromDm(roomid, textInChan)
	go TextInFromHTTP(":9010", "/", textInChan)

	// in -> filter -> in
	textInFiltered := ChineseFilter4TextIn.FilterTextIn(textInChan)

	// in -> chatbot -> out
	chatbot := NewMusharingChatbot("http://localhost:8080")
	go TextOutFromChatbot(chatbot, textInFiltered, textOutChan)

	// out -> filter -> out
	textOutFiltered := ChineseFilter4TextOut.FilterTextOut(textOutChan)

	// out -> (live2d) & (say) & (stdout)
	live2d := NewLive2DDriver(live2dDriverAddr)
	sayer := NewSayer()
	for {
		textOut := <-textOutFiltered

		if textOut == nil {
			continue
		}

		fmt.Println(textOut)
		live2d.TextOutToLive2DDriver(textOut)
		sayer.Say(*textOut)
	}
}
