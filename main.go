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
	"sync"

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

	saying := sync.Mutex{}

	var sayOptions []SayerOption
	if *sayerAudioDevice != "" {
		sayOptions = append(sayOptions, WithAudioDevice(*sayerAudioDevice))
	}
	sayer := NewSayer(sayOptions...)

	// (dm) & (http) -> in
	go TextInFromDm(*roomid, textInChan, WithBlivedmServer(*blivedmServerAddr))
	go TextInFromHTTP(*textInHttpAddr, "/", textInChan)

	// in -> filter -> in
	textInFiltered := textInChan
	// textInFiltered = ChineseFilter4TextIn.FilterTextIn(textInFiltered)
	textInFiltered = NewPriorityReduceFilter(*reduceDuration).FilterTextIn(textInFiltered)
	textInFiltered = TextFilterFunc(func(text string) bool {
		live2dToMotion("flick_head") // 准备张嘴说话
		go func() {
			time.Sleep(time.Second*1 + *reduceDuration) // 提问和回答压到一起，经验值: chatgpt 请求时延 + textout 过滤周期
			saying.Lock()
			defer saying.Unlock()
			sayer.Say(text)
		}()
		return true
	}).FilterTextIn(textInFiltered)

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
	textOutFiltered = TextFilterFunc(func(text string) bool {
		notTooLong := len(text) < 500
		if !notTooLong {  // toooo loooong
			saying.Lock()
			resp := tooLongResponses[tooLongRespIndex]
			tooLongRespIndex = (tooLongRespIndex + 1) % len(tooLongResponses)
			sayer.Say(resp)
			saying.Unlock()
		}
		return notTooLong
	}).FilterTextOut(textOutFiltered)
	textOutFiltered = NewPriorityReduceFilter(*reduceDuration).FilterTextOut(textOutFiltered)

	// out -> (live2d) & (say) & (stdout)
	live2d := NewLive2DDriver(*live2dDriverAddr)

	for {
		textOut := <-textOutFiltered

		if textOut == nil {
			continue
		}

		fmt.Println(*textOut)
		live2d.TextOutToLive2DDriver(textOut)

		saying.Lock()
		sayer.Say(textOut.Content)
		saying.Unlock()

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

var tooLongRespIndex = 0
var tooLongResponses = []string{
	"哎呀，这个故事太长了，我怕我讲不完。",
	"对不起啊，这个事情太长了，让我想起了小时候听外婆给我讲故事，太多情节了，我不会讲。",
	"噫，这个话题真的很长，我的嘴唇已经准备要罢工了。",
	"呜呜呜，这个事情太长了，我怕我讲到天荒地老。",
	"哎呀呀，这个问题太长了，我都能听见我妈妈在背后催促我赶紧去睡觉了，所以我不能讲太多啦。",
	"啊，这个故事的情节和人物都好多，我怕我讲起来太久了，你都听累了。",
	"哟，这个事情真的太长了，我已经可以预见到我们明天都还在讲这个话题，哈哈哈。",
	"嘘嘘，这个问题真的好长好长，让我们下次见面再聊吧，好不好？",
	"抱歉这个太长了，我不能说。",
}

