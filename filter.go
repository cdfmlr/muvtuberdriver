package main

import (
	"log"
	"strings"
	"unicode"
)

type TextInFilter interface {
	FilterTextIn(chIn chan *TextIn) (chOut chan *TextIn)
}

type TextOutFilter interface {
	FilterTextOut(chIn chan *TextOut) (chOut chan *TextOut)
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

func (f TextFilterFunc) FilterTextIn(chIn chan *TextIn) (chOut chan *TextIn) {
	return filterTextChan(chIn, f, func(textIn *TextIn) string {
		if textIn == nil {
			return ""
		}
		return textIn.Content
	})
}

func (f TextFilterFunc) FilterTextOut(chIn chan *TextOut) (chOut chan *TextOut) {
	return filterTextChan(chIn, f, func(textOut *TextOut) string {
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
			log.Printf("chineseFilter true: %s", text)
			return true
		}
	}
	log.Printf("chineseFilter false: %s", text)
	return false
}

var ChineseFilter4TextIn TextInFilter = TextFilterFunc(chineseFilter)
var ChineseFilter4TextOut TextOutFilter = TextFilterFunc(chineseFilter)
