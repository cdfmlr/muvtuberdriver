package sayer

// Sayer is the simple sayer interface for muggles.
// Sayer does blocking & mutex Say().
type Sayer interface {
	// Say text.
	Say(text string) error
}
