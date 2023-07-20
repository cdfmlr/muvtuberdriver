package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"muvtuberdriver/audio"
	"muvtuberdriver/chatbot"
	"muvtuberdriver/config"
	"muvtuberdriver/live2d"
	"muvtuberdriver/model"
	"muvtuberdriver/sayer"
	"net/http"
	"os"
	"reflect"

	"golang.org/x/exp/slog"
)

var (
	dryRun = flag.Bool("dryrun", false, "prints config and exit.")

	genExampleConfig = flag.Bool("gen_example_config", false, "generate example config, print to stdout and exit")
	configFile       = flag.String("c", "", "read config file (YAML)")
)

// Config is the global config
var Config = config.UseConfig()

func main() {
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
		os.Exit(1)
	}

	slog.Info("Config loaded:")
	Config.DesensitizedCopy().Write(log.Writer())

	if *dryRun {
		os.Exit(0)
	}

	os.Setenv("COOLDOWN_INTERVAL", fmt.Sprintf("%v", Config.Chatbot.Chatgpt.GetCooldownDuraton()))
	slog.Info("set COOLDOWN_INTERVAL from config value.", "COOLDOWN_INTERVAL", os.Getenv("COOLDOWN_INTERVAL"))

	textInChan := make(chan *model.TextIn, RecvMsgChanBuf)
	textOutChan := make(chan *model.TextOut, RecvMsgChanBuf)

	audioController := audio.NewController()
	go func() {
		log.Fatal(http.ListenAndServe(
			Config.Listen.AudioControllerWs,
			audioController.WsHandler()))
	}()

	live2d := live2d.NewDriver(Config.Live2d.Driver, Config.Live2d.Forwarder)

	sayer := sayer.NewAllInOneSayer(Config.Sayer.Server, Config.Sayer.Role, audioController, live2d)

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
		live2d.Live2dToMotion("flick_head") // 准备张嘴说话
		sayer.Say(text)
		return true
	}).FilterTextIn(textInFiltered)

	// in -> chatbot -> out
	pchatbot, err := initPrioritizedChatbot()
	if err != nil {
		log.Fatal(err)
	}
	go chatbot.TextOutFromChatbot(pchatbot, textInFiltered, textOutChan)

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
type initChatbotFunc func() (chatbot.Chatbot, error)

// initPrioritizedChatbot initializes a prioritized chatbot with all configured chatbots.
//
// It logs the error and continue if a chatbot fails to initialize.
func initPrioritizedChatbot() (chatbot.Chatbot, error) {
	var chatbots []chatbot.Chatbot

	// 按照优先级 从低到高 依次加入 chatbots

	initChatbotFuncs := []initChatbotFunc{
		initMusharingChatbot,
		initChatgptChatbot,
	}
	for _, initChatbotFunc := range initChatbotFuncs {
		bot, err := initChatbotFunc()
		if err != nil {
			slog.Error("init chatbot failed",
				"initChatbotFunc", reflect.TypeOf(initChatbotFunc).Name(),
				"err", err)
			continue
		}
		if bot != nil {
			chatbots = append(chatbots, bot)
		}
	}

	// 按照前面 append 的顺序，index -> priority 从低到高
	// 组成 prioritizedChatbot。

	chatbotMap := map[model.Priority]chatbot.Chatbot{}
	for i, bot := range chatbots {
		chatbotMap[model.Priority(i)] = bot
	}
	prioritizedChatbot := chatbot.NewPrioritizedChatbot(chatbotMap)
	return prioritizedChatbot, nil
}

// initMusharingChatbot initializes a musharing chatbot if configured.
//
// This function directly reads the global Config.
func initMusharingChatbot() (chatbot.Chatbot, error) {
	enabled, err := Config.Chatbot.Musharing.IsEnabledAndValid()
	if !enabled {
		slog.Info("musharing chatbot is disabled")
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	musharingChatbot, err := chatbot.NewMusharingChatbot(Config.Chatbot.Musharing.Server)
	return musharingChatbot, err
}

// initChatgptChatbot initializes a chatgpt chatbot if configured.
//
// This function directly reads the global Config.
func initChatgptChatbot() (chatbot.Chatbot, error) {
	cfg := Config.Chatbot.Chatgpt

	enabled, err := cfg.IsEnabledAndValid()
	if !enabled {
		slog.Info("chatgpt chatbot is disabled")
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	chatgptChatbot, err := chatbot.NewChatGPTChatbot(
		cfg.Server, cfg.GetCooldownDuraton(), cfg.Configs...)

	return chatgptChatbot, err
}
