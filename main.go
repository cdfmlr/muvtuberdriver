package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	chatbot2 "muvtuberdriver/chatbot"
	"muvtuberdriver/model"
	"net/http"
	"strings"
	"sync"

	// "log"
	"time"

	"golang.org/x/exp/slog"
)

// TODO: 参数已经太长了，必须上配置文件！

var (
	blivedmServerAddr    = flag.String("blivedm", "ws://localhost:12450/api/chat", "(dial) blivedm server address")
	roomid               = flag.Int("roomid", 0, "blivedm roomid")
	live2dDriverAddr     = flag.String("live2ddrv", "http://localhost:9004/driver", "(dail) live2d driver address")
	musharingChatbotAddr = flag.String("mchatbot", "localhost:50051", "(dail) musharing chatbot api server (gRPC) address")
	textInHttpAddr       = flag.String("textinhttp", ":9010", "(listen) textIn http server address")
	textOutHttpAddr      = flag.String("textouthttp", "", "(dail) send textOut to http server (e.g. http://localhost:51080)")
	chatgptAddr          = flag.String("chatgpt", "localhost:50052", "(dail) chatgpt api server (gRPC) address")
	chatgptConfigs       = chatgptConfig{}
	reduceDuration       = flag.Duration("reduce_duration", 2*time.Second, "reduce duration")

	live2dMsgFwd = flag.String("live2d_msg_fwd", "http://localhost:9002/live2d", "(dail) live2d message forward from http")
	readDm       = flag.Bool("readdm", true, "read comment?")
	dropHttpOut  = flag.Int("drophttpout", 5, "textOutHttp drop rate: 0~100")

	// new sayer as a srv & audioview
	audioControllerAddr = flag.String("audiocontroller", ":9020", "(listen) audio controller ws server address")
	sayerAddr           = flag.String("sayer", "localhost:51055", "(dial) sayer gRPC server address")
	sayerRole           = flag.String("sayerrole", "", "role to sayer")
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

	audioController := NewAudioController()
	go func() {
		log.Fatal(http.ListenAndServe(
			*audioControllerAddr,
			audioController.WsHandler()))
	}()

	saying := sync.Mutex{}
	sayer := NewSayer(*sayerAddr, *sayerRole, audioController)
	say := func(text string) {
		defer slog.Info("say: done", "text", text)
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}

		saying.Lock()
		defer saying.Unlock()

		live2dToMotion("flick_head") // 准备张嘴说话
		defer live2dToMotion("idle") // 说完闭嘴

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		ch, err := sayer.Say(ctx, text)
		if err != nil {
			slog.Warn("say failed", "err", err, "text", text)
			return
		}
		for r := range ch {
			switch r {
			case StatusEnd:
				return
			}
		}
	}

	// (dm) & (http) -> in
	if *roomid != 0 {
		go TextInFromDm(*roomid, textInChan, WithBlivedmServer(*blivedmServerAddr))
	}
	if *textInHttpAddr != "" {
		go TextInFromHTTP(*textInHttpAddr, "/", textInChan)
	}

	// in -> filter -> in
	textInFiltered := textInChan
	// textInFiltered = ChineseFilter4TextIn.FilterTextIn(textInFiltered)
	textInFiltered = NewPriorityReduceFilter(*reduceDuration).FilterTextIn(textInFiltered)

	// read dm
	textInFiltered = TextFilterFunc(func(text string) bool {
		if !*readDm {
			return true
		}
		live2dToMotion("flick_head") // 准备张嘴说话
		say(text)
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

	// too long: no say
	textOutFiltered = TextFilterFunc(func(text string) bool {
		notTooLong := len(text) < 800
		if !notTooLong { // toooo loooong
			log.Println("too long, drop it:", ellipsis(text, 30))
			resp := tooLongResponses[tooLongRespIndex]
			tooLongRespIndex = (tooLongRespIndex + 1) % len(tooLongResponses)
			say(resp)
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

		say(textOut.Content)

		if *textOutHttpAddr != "" {
			if rand.Intn(100) >= *dropHttpOut {
				TextOutToHttp(*textOutHttpAddr, textOut)
			} else {
				log.Println("random drop textOut to http")
			}
		}
	}
}

// ellipsis long s -> "front...end"
func ellipsis(s string, n int) string {
	r := []rune(s)

	if len(r) <= n {
		return s
	}

	n -= 3
	h := n / 2

	var sb strings.Builder
	sb.WriteString(string(r[:h]))
	sb.WriteString("...")
	sb.WriteString(string(r[len(r)-h:]))

	return sb.String()
}

// live2dToIdle is a hardcoded quick fix to "live2d不说话的时候也在动嘴"
//
// TODO: 有空好好写一下。
//
//	curl -X POST localhost:9002/live2d -H 'Content-Type: application/json' -d '{"motion": "idle"}'
func live2dToMotion(motion string) {
	client := &http.Client{}
	var data = strings.NewReader(fmt.Sprintf(`{"motion": "%s"}`, motion))
	req, err := http.NewRequest("POST", *live2dMsgFwd, data)
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
