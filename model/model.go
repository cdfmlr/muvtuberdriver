package model

type Text struct {
	Author   string   `json:"author"`
	Content  string   `json:"content"`
	Priority Priority `json:"priority"`
}

// TextIn 是 vtuber 看到的消息
type TextIn = Text

// TextOut 是要给 vtuber 说的东西
type TextOut = Text

type Priority int

const (
	PriorityLow     Priority = 0
	PriorityNormal  Priority = 1
	PriorityHigh    Priority = 2
	PriorityHighest Priority = PriorityHigh
)
