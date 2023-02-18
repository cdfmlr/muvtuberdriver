package main

import (
	"flag"
	"fmt"
	"time"
)

var (
	blivedmServerAddr    = flag.String("blivedm", "ws://localhost:12450/api/chat", "blivedm server address")
	roomid               = flag.Int("roomid", 0, "blivedm roomid")
	live2dDriverAddr     = flag.String("live2ddrv", "http://localhost:9004/driver", "live2d driver address")
	musharingChatbotAddr = flag.String("mchatbot", "http://localhost:8080", "musharing chatbot api server address")
	textInHttpAddr       = flag.String("textinhttp", ":9010", "textIn http server address")
	chatgptAddr          = flag.String("chatgpt", "http://localhost:9006", "chatgpt api server address")
	chatgptAccessToken   = flag.String("chatgpt_access_token", "", "chatgpt access token")
	chatgptPrompt        = flag.String("chatgpt_prompt", "", "chatgpt prompt")
	reduceDuration       = flag.Duration("reduce_duration", 2*time.Second, "reduce duration")
)

func main() {
	flag.Parse()

	textInChan := make(chan *TextIn, RecvMsgChanBuf)
	textOutChan := make(chan *TextOut, RecvMsgChanBuf)

	// (dm) & (http) -> in
	go TextInFromDm(*roomid, textInChan, WithBlivedmServer(*blivedmServerAddr))
	go TextInFromHTTP(*textInHttpAddr, "/", textInChan)

	// in -> filter -> in
	textInFiltered := ChineseFilter4TextIn.FilterTextIn(textInChan)
	textInFiltered = NewPriorityReduceFilter(*reduceDuration).FilterTextIn(textInFiltered)

	// in -> chatbot -> out
	chatbot := NewPrioritizedChatbot(map[Priority]Chatbot{
		PriorityLow:  NewMusharingChatbot(*musharingChatbotAddr),
		PriorityHigh: NewChatGPTChatbot(*chatgptAddr, *chatgptAccessToken, *chatgptPrompt),
	})
	go TextOutFromChatbot(chatbot, textInFiltered, textOutChan)

	// out -> filter -> out
	textOutFiltered := ChineseFilter4TextOut.FilterTextOut(textOutChan)
	textOutFiltered = NewPriorityReduceFilter(*reduceDuration).FilterTextOut(textOutFiltered)

	// out -> (live2d) & (say) & (stdout)
	live2d := NewLive2DDriver(*live2dDriverAddr)
	sayer := NewSayer()
	for {
		textOut := <-textOutFiltered

		if textOut == nil {
			continue
		}

		fmt.Println(*textOut)
		live2d.TextOutToLive2DDriver(textOut)
		sayer.Say(textOut.Content)
	}
}
