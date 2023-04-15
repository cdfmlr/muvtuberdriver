package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	chatbot2 "muvtuberdriver/chatbot"
	"muvtuberdriver/config"
	"muvtuberdriver/model"
	"net/http"
	"os"
	"reflect"
	"time"

	"golang.org/x/exp/slog"
)

var tipsFlagsToConfig = `目前这是个过度版本：
我不确定是否要完全弃用 flags 又或者现在新的 config from yaml 机制是否合理，所以两个都保留了。
我现在只是把原来的 flags 嫁接到了 config 上，测试一个版本，如果 config 机制没有问题，就删掉 flags。
现在处理的逻辑是这样：如果设置了 -c 则使用配置文件，否则使用 flags。`

var (
	// ⬇️ deprecated: use config file instead ⬇️
	// TODO: remove these flags at v0.4.0
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
	// ⬆️ deprecated: use config file instead ⬆️

	genExampleConfig = flag.Bool("gen_example_config", false, "generate example config, print to stdout and exit")
	configFile       = flag.String("c", "", "read config file (YAML)")
)

// Config is the global config
var Config = config.UseConfig()

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
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Println("notice:\n", tipsFlagsToConfig)
	}
	flag.Parse()

	if *genExampleConfig {
		c := config.ExampleConfig()
		c.Write(os.Stdout)
		return
	}

	// TODO: remove flags->config logic at v0.4.0
	if *configFile != "" {
		slog.Info("Reading config file.", "configFile", *configFile)
		Config.ReadFromYaml(*configFile)
	} else {
		slog.Error("Config file is required, use -c path/to/config.yaml to specify")
		// os.Exit(1)
		slog.Warn("Using cli flags is deprecated. Support will be removed in the future (about 2022-05-01, v0.4.0)")
		flagToConfig()
	}
	slog.Info("Config loaded:")
	Config.Write(log.Writer())
	// os.Exit(0)

	os.Setenv("COOLDOWN_INTERVAL", fmt.Sprintf("%v", Config.Chatbot.Chatgpt.GetCooldownDuraton()))
	slog.Info("set COOLDOWN_INTERVAL from config value.", "COOLDOWN_INTERVAL", os.Getenv("COOLDOWN_INTERVAL"))

	textInChan := make(chan *model.TextIn, RecvMsgChanBuf)
	textOutChan := make(chan *model.TextOut, RecvMsgChanBuf)

	audioController := NewAudioController()
	go func() {
		log.Fatal(http.ListenAndServe(
			Config.Listen.AudioControllerWs,
			audioController.WsHandler()))
	}()

	live2d := NewLive2DDriver(Config.Live2d.Driver, Config.Live2d.Forwarder)

	sayer := NewAllInOneSayer(Config.Sayer.Server, Config.Sayer.Role, audioController, live2d)

	// (dm) & (http) -> in
	if Config.Blivedm.Roomid != 0 {
		go TextInFromDm(Config.Blivedm.Roomid, textInChan, WithBlivedmServer(Config.Blivedm.Server))
	}
	if Config.Listen.TextInHttp != "" {
		go TextInFromHTTP(Config.Listen.TextInHttp, "/", textInChan)
	}

	// in -> filter -> in
	textInFiltered := textInChan
	// textInFiltered = ChineseFilter4TextIn.FilterTextIn(textInFiltered)
	textInFiltered = NewPriorityReduceFilter(Config.GetReduceDuration()).FilterTextIn(textInFiltered)

	// read dm
	textInFiltered = TextFilterFunc(func(text string) bool {
		if !Config.ReadDm {
			return true
		}
		live2d.live2dToMotion("flick_head") // 准备张嘴说话
		sayer.Say(text)
		return true
	}).FilterTextIn(textInFiltered)

	// in -> chatbot -> out
	pchatbot, err := initPrioritizedChatbot()
	if err != nil {
		log.Fatal(err)
	}
	go chatbot2.TextOutFromChatbot(pchatbot, textInFiltered, textOutChan)

	// out -> filter -> out
	textOutFiltered := textOutChan
	// textOutFiltered := ChineseFilter4TextOut.FilterTextOut(textOutChan)

	// too long: no say
	tooLongFilter := NewTooLongFilter(Config.TooLong.MaxWords, Config.TooLong.Quibbles)
	textOutFiltered = tooLongFilter.TextFilterFunc(func(text, quibble *string) {
		if quibble != nil {
			sayer.Say(*quibble)
		} else {
			sayer.Say("这是禁止事项。")
		}
	}).FilterTextOut(textOutFiltered)

	textOutFiltered = NewPriorityReduceFilter(Config.GetReduceDuration()).FilterTextOut(textOutFiltered)

	// out -> (live2d) & (say) & (stdout)

	for {
		textOut := <-textOutFiltered

		if textOut == nil {
			continue
		}

		// fmt.Println(*textOut)
		slog.Info("[textOut]",
			"author", textOut.Author,
			"priority", textOut.Priority,
			"content", textOut.Content)

		live2d.TextOutToLive2DDriver(textOut)

		sayer.Say(textOut.Content)

		if Config.TextOutHttp.Server != "" {
			if rand.Intn(100) >= Config.TextOutHttp.DropRate {
				TextOutToHttp(Config.TextOutHttp.Server, textOut)
			} else {
				slog.Info("[TextOutHttp] random drop textOut.")
			}
		}
	}
}

// initChatbotFunc is a type of function that initializes a chatbot.
//
// A initChatbotFunc should return a chatbot and nil error if it succeeds.
// If it fails, it should return nil and a non-nil error.
//
// A initChatbotFunc reads config from the global Config variable.
type initChatbotFunc func() (chatbot2.Chatbot, error)

// initPrioritizedChatbot initializes a prioritized chatbot with all configured chatbots.
//
// It logs the error and continue if a chatbot fails to initialize.
func initPrioritizedChatbot() (chatbot2.Chatbot, error) {
	var chatbots []chatbot2.Chatbot

	// 按照优先级 从低到高 依次加入 chatbots

	initChatbotFuncs := []initChatbotFunc{
		initMusharingChatbot,
		initChatgptChatbot,
	}
	for _, initChatbotFunc := range initChatbotFuncs {
		chatbot, err := initChatbotFunc()
		if err != nil {
			slog.Error("init chatbot failed",
				"initChatbotFunc", reflect.TypeOf(initChatbotFunc).Name(),
				"err", err)
			continue
		}
		if chatbot != nil {
			chatbots = append(chatbots, chatbot)
		}
	}

	// 按照前面 append 的顺序，index -> priority 从低到高
	// 组成 prioritizedChatbot。

	chatbotMap := map[model.Priority]chatbot2.Chatbot{}
	for i, chatbot := range chatbots {
		chatbotMap[model.Priority(i)] = chatbot
	}
	prioritizedChatbot := chatbot2.NewPrioritizedChatbot(chatbotMap)
	return prioritizedChatbot, nil
}

// initMusharingChatbot initializes a musharing chatbot if configured.
//
// This function directly reads the global Config.
func initMusharingChatbot() (chatbot2.Chatbot, error) {
	enabled, err := Config.Chatbot.Musharing.IsEnabledAndValid()
	if !enabled {
		slog.Info("musharing chatbot is disabled")
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	musharingChatbot, err := chatbot2.NewMusharingChatbot(Config.Chatbot.Musharing.Server)
	return musharingChatbot, err
}

// initChatgptChatbot initializes a chatgpt chatbot if configured.
//
// This function directly reads the global Config.
func initChatgptChatbot() (chatbot2.Chatbot, error) {
	enabled, err := Config.Chatbot.Chatgpt.IsEnabledAndValid()
	if !enabled {
		slog.Info("chatgpt chatbot is disabled")
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	chatgptChatbot, err := chatbot2.NewChatGPTChatbot(Config.Chatbot.Chatgpt.Server, Config.Chatbot.Chatgpt.Configs)
	return chatgptChatbot, err
}

// Deprecated: use config file instead.
// flagToConfig read flags and set config
//
// TODO: remove this function in v0.4.0
func flagToConfig() {
	if *blivedmServerAddr != "" {
		Config.Blivedm.Server = *blivedmServerAddr
	}
	if *roomid != 0 {
		Config.Blivedm.Roomid = *roomid
	}
	if *live2dDriverAddr != "" {
		Config.Live2d.Driver = *live2dDriverAddr
	}
	if *musharingChatbotAddr != "" {
		Config.Chatbot.Musharing.Server = *musharingChatbotAddr
	}
	if *textInHttpAddr != "" {
		Config.Listen.TextInHttp = *textInHttpAddr
	}
	if *textOutHttpAddr != "" {
		Config.TextOutHttp.Server = *textOutHttpAddr
	}
	if *chatgptAddr != "" {
		Config.Chatbot.Chatgpt.Server = *chatgptAddr
	}
	if len([]chatbot2.ChatGPTConfig(chatgptConfigs)) != 0 {
		Config.Chatbot.Chatgpt.Configs = chatgptConfigs
	}
	if *reduceDuration != 0 {
		Config.ReduceDuration = int(reduceDuration.Seconds())
	}
	if *live2dMsgFwd != "" {
		Config.Live2d.Forwarder = *live2dMsgFwd
	}
	if *readDm != false {
		Config.ReadDm = *readDm
	}
	if *dropHttpOut != 0 {
		Config.TextOutHttp.DropRate = *dropHttpOut
	}
	if *audioControllerAddr != "" {
		Config.Listen.AudioControllerWs = *audioControllerAddr
	}
	if *sayerAddr != "" {
		Config.Sayer.Server = *sayerAddr
	}
	if *sayerRole != "" {
		Config.Sayer.Role = *sayerRole
	}
}
