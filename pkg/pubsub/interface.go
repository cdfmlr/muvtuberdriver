package pubsub

// PubSub[T] is the pub/sub interface.
type PubSub[T any] interface {
	Publish(payload T) error
	Subscribe() <-chan Result[T]
}

// Result[T] is the result of a PubSub[T] subscription.
type Result[T any] struct {
	Ok  T
	Err error
}
