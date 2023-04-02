package musayerapi

// Sayer is the interface that wraps the Say method.
//
// Implementations of Sayer can be used to ServeGrpc.
type Sayer interface {
	// Say converts text to speech and returns the audio file.
	// Returns the format of the audio file and the audio file
	// content.
	Say(role string, text string) (format string, audio []byte, err error)
}
