package chatbot

import (
	"log"
	"muvtuberdriver/model"
)

type Chatbot interface {
	Chat(textIn *model.TextIn) (*model.TextOut, error)
}

func TextOutFromChatbot(chatbot Chatbot, textInChan <-chan *model.TextIn, textOutChan chan<- *model.TextOut) {
	for {
		textIn := <-textInChan
		textOut, err := chatbot.Chat(textIn)
		if err != nil {
			log.Printf("chatbot.Chat(%v) failed: %v", textIn, err)
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
	log.Printf("[PrioritizedChatbot] Chat(%s): %s", textIn.Author, textIn.Content)

	priority := textIn.Priority

	for i := priority; i >= 0; i-- {
		chatbot, ok := p.chatbots[i]
		if !ok {
			continue
		}

		textOut, err := chatbot.Chat(textIn)
		if err != nil {
			if i == 0 {
				log.Printf("PrioritizedChatbot all Chatbots failed: %v, return nil", err)
				return nil, err
			} else {
				log.Printf("%T.Chat(%v) failed: %v, try next chatbot", chatbot, textIn, err)
				continue
			}
		}
		if textOut != nil {
			log.Printf("[PrioritizedChatbot] Chat(%s): %s => (%s): %s", textIn.Author, textIn.Content, textOut.Author, textOut.Content)
			return textOut, nil
		}
	}

	return nil, nil
}

func NewPrioritizedChatbot(chatbots map[model.Priority]Chatbot) Chatbot {
	return &PrioritizedChatbot{
		chatbots: chatbots,
	}
}

// endregion PrioritizedChatbot
