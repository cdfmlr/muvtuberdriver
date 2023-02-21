package main

import (
	"log"
	"muvtuberdriver/model"
	"strings"
	"sync"
	"time"
	"unicode"
)

type TextInFilter interface {
	FilterTextIn(chIn chan *model.TextIn) (chOut chan *model.TextIn)
}

type TextOutFilter interface {
	FilterTextOut(chIn chan *model.TextOut) (chOut chan *model.TextOut)
}

// TextFilterFunc 对字符串 text 进行过滤，返回 true 表示保留，false 则滤掉。
type TextFilterFunc func(text string) bool

// filterTextChan 是一个通用的过滤器，可以过滤 TextIn 或 TextOut 或者其他任意可以转化为字符串的东西。
// key 用于从 chIn 传出的 T 中提取 string，交给 f 进行过滤。
func filterTextChan[T any](chIn chan T, f TextFilterFunc, key func(T) string) (chOut chan T) {
	chOut = make(chan T, RecvMsgChanBuf)
	go func() {
		for in := range chIn {
			if f(key(in)) {
				chOut <- in
			}
		}
	}()
	return chOut
}

func (f TextFilterFunc) FilterTextIn(chIn chan *model.TextIn) (chOut chan *model.TextIn) {
	return filterTextChan(chIn, f, func(textIn *model.TextIn) string {
		if textIn == nil {
			return ""
		}
		return textIn.Content
	})
}

func (f TextFilterFunc) FilterTextOut(chIn chan *model.TextOut) (chOut chan *model.TextOut) {
	return filterTextChan(chIn, f, func(textOut *model.TextOut) string {
		if textOut == nil {
			return ""
		}
		return textOut.Content
	})
}

func noFilter(text string) bool {
	return true
}

var NoFilter4TextIn TextInFilter = TextFilterFunc(noFilter)
var NoFilter4TextOut TextOutFilter = TextFilterFunc(noFilter)

// chineseFilter 只允许中文的 text
func chineseFilter(text string) bool {
	text = strings.TrimSpace(text)
	for _, c := range text {
		if unicode.Is(unicode.Han, c) {
			// log.Printf("chineseFilter true: %s", text)
			return true
		}
	}
	// log.Printf("chineseFilter false: %s", text)
	return false
}

var ChineseFilter4TextIn TextInFilter = TextFilterFunc(chineseFilter)
var ChineseFilter4TextOut TextOutFilter = TextFilterFunc(chineseFilter)

// PriorityReduceFilter 每 duration 的时间，从收到的消息中选出一些作为输出。
//
// 选择的标准是 Priority 最高的。如果最高 Priority 有多条：
// 1. 如果这些消息的 Priority 为 PriorityHighest 则输出所有这些消息；
// 2. 否则，输出其中 Content 字数最多的一条；
type PriorityReduceFilter struct {
	temp     []*model.TextIn
	mu       sync.RWMutex
	duration time.Duration
}

func NewPriorityReduceFilter(duration time.Duration) *PriorityReduceFilter {
	return &PriorityReduceFilter{
		temp:     make([]*model.TextIn, 0, 10),
		duration: duration,
	}
}

func (f *PriorityReduceFilter) FilterTextIn(chIn chan *model.TextIn) (chOut chan *model.TextIn) {
	return f.filter(chIn)
}

func (f *PriorityReduceFilter) FilterTextOut(chIn chan *model.TextOut) (chOut chan *model.TextOut) {
	return f.filter(chIn)
}

func (f *PriorityReduceFilter) filter(chIn chan *model.Text) (chOut chan *model.Text) {
	chOut = make(chan *model.TextOut, RecvMsgChanBuf)
	go func() {
		timeout := time.NewTicker(f.duration)

		for {
			select {
			case in := <-chIn:
				f.mu.Lock()
				f.temp = append(f.temp, in)
				f.mu.Unlock()
			case <-timeout.C:
				f.outputMaxPriorityOnes(chOut)

				f.mu.Lock()
				f.temp = f.temp[:0]
				f.mu.Unlock()
			}
		}
	}()

	return chOut
}

// selectOneInTemp 找出 temp 中最高的 Priority 。
func (f *PriorityReduceFilter) maxPriorityInTemp() model.Priority {
	var max model.Priority

	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, t := range f.temp {
		if t.Priority >= max {
			max = t.Priority
		}
	}
	return max
}

func (f *PriorityReduceFilter) maxContentLengthInTemp() (maxLen int, index int) {
	var max, idx int

	f.mu.RLock()
	defer f.mu.RUnlock()

	for i, t := range f.temp {
		if len(t.Content) > max {
			max = len(t.Content)
			idx = i
		}
	}
	return max, idx
}

func (f *PriorityReduceFilter) outputMaxPriorityOnes(chOut chan<- *model.Text) {
	f.mu.RLock()
	switch len(f.temp) {
	case 0:
		f.mu.RUnlock()
		return
	case 1:
		t := f.temp[0]
		f.mu.RUnlock()

		f.mu.Lock()
		f.temp = f.temp[:0]
		f.mu.Unlock()

		if t == nil {
			return
		}

		t.Priority = model.PriorityHighest // 消息少，提权，以求高质量 Chatbot 回复
		log.Printf("PriorityReduceFilter outputMaxPriorityOnes [Priority -> Highest]: %+v", t)
		chOut <- t
		return
	default:
		f.mu.RUnlock()
	}

	maxPriority := f.maxPriorityInTemp()

	choosen := make([]*model.Text, 1)

	f.mu.RLock()
	for _, t := range f.temp {
		if t.Priority == maxPriority {
			if t.Priority > model.PriorityHighest {
				t.Priority = model.PriorityHighest // write
			}
			choosen = append(choosen, t)
		}
	}
	f.mu.RUnlock()

	if maxPriority == model.PriorityHighest {
		// 如果这些消息的 Priority >= PriorityHighest 则输出所有这些消息；
		for _, t := range choosen {
			log.Printf("PriorityReduceFilter outputMaxPriorityOnes: %+v", t)
			chOut <- t
		}
	} else {
		// 否则，输出其中 Content 字数最多的一条；
		maxLen, maxLenIdx := maxLenOfTextInSlice(choosen)
		if maxLen <= 0 {
			return
		}

		one := choosen[maxLenIdx]
		log.Printf("PriorityReduceFilter outputMaxPriorityOnes: %+v", one)
		chOut <- one
	}
}

func maxLenOfTextInSlice(slice []*model.Text) (maxLen int, index int) {
	if len(slice) == 0 {
		return 0, 0
	}
	for i, t := range slice {
		if t == nil {
			continue
		}
		if len(t.Content) > maxLen {
			maxLen = len(t.Content)
			index = i
		}
	}
	return maxLen, index
}
