package main

// TextIn 是 vtuber 看到的消息
type TextIn struct {
	Author  string `json:"author"`
	Content string `json:"content"`
}

// TextOut 是要给 vtuber 说的东西
type TextOut = string
