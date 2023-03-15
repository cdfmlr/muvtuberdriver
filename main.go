package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	chatbot2 "muvtuberdriver/chatbot"
	"muvtuberdriver/model"
	"net/http"
	"strings"

	// "log"
	"time"
)

// TODO: 参数已经太长了，必须上配置文件！

var (
	blivedmServerAddr    = flag.String("blivedm", "ws://localhost:12450/api/chat", "blivedm server address")
	roomid               = flag.Int("roomid", 0, "blivedm roomid")
	live2dDriverAddr     = flag.String("live2ddrv", "http://localhost:9004/driver", "live2d driver address")
	musharingChatbotAddr = flag.String("mchatbot", "localhost:50051", "musharing chatbot api server (gRPC) address")
	textInHttpAddr       = flag.String("textinhttp", ":9010", "textIn http server address")
	chatgptAddr          = flag.String("chatgpt", "localhost:50052", "chatgpt api server (gRPC) address")
	chatgptConfigs       = chatgptConfig{}
	reduceDuration       = flag.Duration("reduce_duration", 2*time.Second, "reduce duration")
	sayerAudioDevice     = flag.String("audio_device", "", "sayer audio device. Run <say -a '?'> to get the list of audio devices. Pass the number of the audio device you want to use. . (Default: system sound output)")
)

type chatgptConfig []chatbot2.ChatGPTConfig

// String returns the default value (大概吧)
func (i *chatgptConfig) String() string {
	return ""
}

func (i *chatgptConfig) Set(value string) error {
	return json.Unmarshal([]byte(value), &i)
}

func main() {
	flag.Var(&chatgptConfigs, "chatgpt_config", `chatgpt configs in json: [{"version": 3, "api_key": "sk_xxx", "initial_prompt": "hello"}, ...] `)
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
	chatgptChatbot, err := chatbot2.NewChatGPTChatbot(*chatgptAddr, chatgptConfigs)
	if err != nil {
		log.Fatal(err)
	}
	chatbot := chatbot2.NewPrioritizedChatbot(map[model.Priority]chatbot2.Chatbot{
		model.PriorityLow:  musharingChatbot,
		model.PriorityHigh: chatgptChatbot,
	})
	go chatbot2.TextOutFromChatbot(chatbot, textInFiltered, textOutChan)

	// out -> filter -> out
	textOutFiltered := textOutChan
	// textOutFiltered := ChineseFilter4TextOut.FilterTextOut(textOutChan)
	textOutFiltered = NewPriorityReduceFilter(*reduceDuration).FilterTextOut(textOutFiltered)

	// out -> (live2d) & (say) & (stdout)
	live2d := NewLive2DDriver(*live2dDriverAddr)

	var sayOptions []SayerOption
	if *sayerAudioDevice != "" {
		sayOptions = append(sayOptions, WithAudioDevice(*sayerAudioDevice))
	}
	sayer := NewSayer(sayOptions...)

	for {
		textOut := <-textOutFiltered

		if textOut == nil {
			continue
		}

		live2dToMotion("flick_head") // 张嘴说话

		fmt.Println(*textOut)
		live2d.TextOutToLive2DDriver(textOut)
		sayer.Say(textOut.Content)

		live2dToMotion("idle") // 说完闭嘴
	}
}

// live2dToIdle is a hardcoded quick fix to "live2d不说话的时候也在动嘴"
//
// TODO: 有空好好写一下。
//
//	curl -X POST localhost:9002/live2d -H 'Content-Type: application/json' -d '{"motion": "idle"}'
func live2dToMotion(motion string) {
	client := &http.Client{}
	var data = strings.NewReader(fmt.Sprintf(`{"motion": "%s"}`, motion))
	req, err := http.NewRequest("POST", "http://localhost:9002/live2d", data)
	if err != nil {
		log.Printf("[toIdle] http.NewRequest failed. err=%v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[toIdle] http client do request failed. err=%v", err)
	}
	defer resp.Body.Close()
	// bodyText, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Printf("%s\n", bodyText)
}
