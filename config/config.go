package config

import (
	"io"
	chatbot2 "muvtuberdriver/chatbot"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// XXX: 我懒得写默认值了，就务必全部填写吧
// XXX: viper 也感觉太重了，就只支持 yaml 吧

type config struct {
	Blivedm     BlivedmConfig     // 获取弹幕
	TextOutHttp TextOutHttpConfig // 文本输出发送给 http 服务器
	Live2d      Live2dConfig      // live2dDriver
	Chatbot     ChatbotConfig     // 聊天机器人
	Sayer       SayerConfig       // 文本语音合成
	Listen      ListenConfig      // 这个程序会监听的一些地址

	// ⬇️ 杂项

	ReadDm         bool // 复读评论
	ReduceDuration int  // 评论筛选时间间隔 (秒)
	TooLongDontSay int  // 文本太长了，不调用语音合成 (字符数 len(string))
}

// BlivedmConfig 获取弹幕的配置
type BlivedmConfig struct {
	Server string // blivedm server address
	Roomid int    // bilibili live room id
}

// TextOutHttpConfig 文本输出发送给 http 服务器
type TextOutHttpConfig struct {
	Server   string // http server address
	DropRate int    // drop rate of text output: 0~100
}

// Live2dConfig live2d 配置
type Live2dConfig struct {
	Driver    string // live2d driver address
	Forwarder string // live2d websocket message forwarder
}

// ChatbotConfig 聊天机器人配置
type ChatbotConfig struct {
	Musharing MusharingChatbotConfig // chatterbot 配置
	Chatgpt   ChatgptChatbotConfig
}

// MusharingChatbotConfig chatterbot 配置
type MusharingChatbotConfig struct {
	Server string // musharing chatbot api server (gRPC) address
}

// ChatgptChatbotConfig chatgpt 配置
type ChatgptChatbotConfig struct {
	Server   string                   // chatgpt api server (gRPC) address
	Configs  []chatbot2.ChatGPTConfig // chatgpt configs in json: [{"version": 3, "api_key": "sk_xxx", "initial_prompt": "hello"}, ...]
	Cooldown int                      // chatgpt cooldown time (seconds)
}

func (c *ChatgptChatbotConfig) GetCooldownDuraton() time.Duration {
	return time.Duration(c.Cooldown) * time.Second
}

// SayerConfig 文本语音合成配置
type SayerConfig struct {
	Server string // sayer gRPC server address
	Role   string // role to sayer
}

// ListenConfig 这个程序会监听的一些地址
type ListenConfig struct {
	TextInHttp        string // textIn http server address: 从 http 接收文本输入
	AudioControllerWs string // audio controller ws server address: audioview 通过 websocket 与这个程序通信
}

func (c *config) Read(src io.Reader) error {
	return yaml.NewDecoder(src).Decode(&c)
}

func (c *config) Write(dst io.Writer) error {
	return yaml.NewEncoder(dst).Encode(&c)
}

// ReadFromYaml 读取配置文件
func (c *config) ReadFromYaml(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return c.Read(f)
}

// WriteToYaml 写入配置文件
func (c *config) WriteToYaml(file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return c.Write(f)
}

// Check 检查配置是否合法: 懒得检查了都允许吧
func (c *config) Check() error {
	return nil
}

// GetReduceDuration is a shorthand for:
//
//	time.Duration(c.ReduceDuration) * time.Second
func (c *config) GetReduceDuration() time.Duration {
	return time.Duration(c.ReduceDuration) * time.Second
}

var configInstance = config{}

func UseConfig() *config {
	return &configInstance
}

// ExampleConfig 会生成一个示例配置，返回生成的配置。
func ExampleConfig() config {
	c := config{
		Blivedm: BlivedmConfig{
			Server: "ws://localhost:12450/api/chat",
			Roomid: 123456,
		},
		TextOutHttp: TextOutHttpConfig{
			Server:   "",
			DropRate: 0,
		},
		Live2d: Live2dConfig{
			Driver:    "http://localhost:9004/driver",
			Forwarder: "http://localhost:9002/live2d",
		},
		Chatbot: ChatbotConfig{
			Musharing: MusharingChatbotConfig{
				Server: "localhost:50051",
			},
			Chatgpt: ChatgptChatbotConfig{
				Server: "localhost:50052",
				Configs: []chatbot2.ChatGPTConfig{
					{
						Version:       3,
						ApiKey:        "sk_xxx",
						InitialPrompt: "hello",
					},
				},
				Cooldown: 0,
			},
		},
		Sayer: SayerConfig{
			Server: "localhost:51055",
			Role:   "miku",
		},
		Listen: ListenConfig{
			TextInHttp:        "0.0.0.0:9010",
			AudioControllerWs: "0.0.0.0:9020",
		},
		ReadDm:         true,
		ReduceDuration: 2,
		TooLongDontSay: 1000,
	}

	return c
}