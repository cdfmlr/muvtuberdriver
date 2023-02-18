package main

type Text struct {
	Author   string `json:"author"`
	Content  string `json:"content"`
}

// TextIn 是 vtuber 看到的消息
type TextIn = Text

// TextOut 是要给 vtuber 说的东西
type TextOut = Text
