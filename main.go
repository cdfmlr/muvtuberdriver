package main

import (
	"flag"
	"fmt"
	"log"
	chatbot2 "muvtuberdriver/chatbot"
	"muvtuberdriver/model"

	// "log"
	"time"
)

var (
	blivedmServerAddr    = flag.String("blivedm", "ws://localhost:12450/api/chat", "blivedm server address")
	roomid               = flag.Int("roomid", 0, "blivedm roomid")
	live2dDriverAddr     = flag.String("live2ddrv", "http://localhost:9004/driver", "live2d driver address")
	musharingChatbotAddr = flag.String("mchatbot", "localhost:50051", "musharing chatbot api server address")
	textInHttpAddr       = flag.String("textinhttp", ":9010", "textIn http server address")
	chatgptAddr          = flag.String("chatgpt", "http://localhost:9006", "chatgpt api server address")
	chatgptAccessToken   = flag.String("chatgpt_access_token", "", "chatgpt access token")
	chatgptPrompt        = flag.String("chatgpt_prompt", "", "chatgpt prompt")
	reduceDuration       = flag.Duration("reduce_duration", 2*time.Second, "reduce duration")
)

func main() {
	flag.Parse()

	textInChan := make(chan *model.TextIn, RecvMsgChanBuf)
	textOutChan := make(chan *model.TextOut, RecvMsgChanBuf)

	// (dm) & (http) -> in
	go TextInFromDm(*roomid, textInChan, WithBlivedmServer(*blivedmServerAddr))
	go TextInFromHTTP(*textInHttpAddr, "/", textInChan)

	// in -> filter -> in
	textInFiltered := ChineseFilter4TextIn.FilterTextIn(textInChan)
	textInFiltered = NewPriorityReduceFilter(*reduceDuration).FilterTextIn(textInFiltered)

	// in -> chatbot -> out
	musharingChatbot, err := chatbot2.NewMusharingChatbot(*musharingChatbotAddr)
	if err != nil {
		log.Fatal(err)
	}
	chatbot := chatbot2.NewPrioritizedChatbot(map[model.Priority]chatbot2.Chatbot{
		model.PriorityLow:  musharingChatbot,
		model.PriorityHigh: chatbot2.NewChatGPTChatbot(*chatgptAddr, *chatgptAccessToken, *chatgptPrompt),
	})
	go chatbot2.TextOutFromChatbot(chatbot, textInFiltered, textOutChan)

	// out -> filter -> out
	textOutFiltered := textOutChan
	// textOutFiltered := ChineseFilter4TextOut.FilterTextOut(textOutChan)
	textOutFiltered = NewPriorityReduceFilter(*reduceDuration).FilterTextOut(textOutFiltered)

	// out -> (live2d) & (say) & (stdout)
	live2d := NewLive2DDriver(*live2dDriverAddr)
	sayer := NewSayer(WithAudioDevice("65"))
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
