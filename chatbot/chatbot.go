package chatbot

import (
	"errors"
	"log"
	"muvtuberdriver/model"
	"muvtuberdriver/pkg/ellipsis"
)

type Chatbot interface {
	Chat(textIn *model.TextIn) (*model.TextOut, error)
}

func TextOutFromChatbot(chatbot Chatbot, textInChan <-chan *model.TextIn, textOutChan chan<- *model.TextOut) {
	for {
		textIn := <-textInChan
		textOut, err := chatbot.Chat(textIn)
		if err != nil {
			log.Printf("ERROR chatbot.Chat(%v) failed: %v", textIn, err)
		}
		textOutChan <- textOut
	}
}

// region ChatGPTChatbot

// endregion ChatGPTChatbot

// region PrioritizedChatbot

// PrioritizedChatbot 按照 TextIn 的 Priority 调用 Chatbot。
// 高优先级的 Chatbot 应该是对话质量更高的（例如 ChatGPTChatbot），而低优先级的 Chatbot 用来保底。
// 如果没有对应级别的 Chatbot，会往下滑到更低的级别。
type PrioritizedChatbot struct {
	chatbots map[model.Priority]Chatbot
}

// TODO: timeout -> try others.
func (p *PrioritizedChatbot) Chat(textIn *model.TextIn) (*model.TextOut, error) {
	if textIn == nil {
		return nil, nil
	}
	log.Printf("INFO [PrioritizedChatbot] Chat(%s): %q", textIn.Author, ellipsis.Centering(textIn.Content, 17))

	priority := textIn.Priority

	for i := priority; i >= 0; i-- {
		chatbot, ok := p.chatbots[i]
		if !ok {
			continue
		}

		textOut, err := chatbot.Chat(textIn)
		if err != nil {
			if i == 0 {
				log.Printf("ERROR [PrioritizedChatbot] all Chatbots failed: %v, return nil", err)
				return nil, err
			} else {
				log.Printf("WARN [PrioritizedChatbot] %T.Chat(%v) failed: %v, try next chatbot", chatbot, textIn, err)
				continue
			}
		}
		if textOut != nil {
			// log.Printf("[PrioritizedChatbot] Chat(%s): %s => (%s): %s", textIn.Author, ellipsis.Centering(textIn.Content, 17), textOut.Author, ellipsis.Centering(textOut.Content, 17))
			// 这个作为特例，不用 ellipsis，而是输出完整的内容：
			// 这个是目前唯一一个一行看到完整 输入 -> 输出 的地方，也许可以用来收集数据做训练？
			// 新改成了 %q 的格式，以前会换行，现在不允许了。
			log.Printf("INFO [PrioritizedChatbot] Chat(%s): %q => (%s): %q", textIn.Author, textIn.Content, textOut.Author, textOut.Content)
			return textOut, nil
		}
	}

	return nil, errors.New("no chatbot available")
}

func NewPrioritizedChatbot(chatbots map[model.Priority]Chatbot) Chatbot {
	return &PrioritizedChatbot{
		chatbots: chatbots,
	}
}

// endregion PrioritizedChatbot
