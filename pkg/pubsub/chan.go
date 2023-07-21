package pubsub

// pubSubChan is a PubSub[T] implementation based on go chan.
type pubSubChan[T any] struct {
	pub  chan T
	subs []chan Result[T]
}

const pubSubChanBufSize = 8

func NewPubSubChan[T any]() PubSub[T] {
	ps := &pubSubChan[T]{
		pub:  make(chan T, pubSubChanBufSize),
		subs: []chan Result[T]{},
	}

	go ps.run()

	return ps
}

func (ps *pubSubChan[T]) Publish(payload T) error {
	// no subscribers: drop message to avoid blocking
	if len(ps.subs) != 0 {
		ps.pub <- payload
	}
	return nil
}

func (ps *pubSubChan[T]) Subscribe() <-chan Result[T] {
	ch := make(chan Result[T], pubSubChanBufSize)
	ps.subs = append(ps.subs, ch)
	return ch
}

func (ps *pubSubChan[T]) run() {
	for msg := range ps.pub {
		for _, sub := range ps.subs {
			sub <- Result[T]{Ok: msg}
		}
	}
}
