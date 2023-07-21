package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// pubSubRedis is a PubSub[T] implementation based on
// redis pub/sub and json encoding (TODO: custom encoding).
type pubSubRedis[T any] struct {
	name string
	rdb  *redis.Client
}

func NewPubSubRedis[T any](name string, rdb *redis.Client) PubSub[T] {
	return &pubSubRedis[T]{
		name: name,
		rdb:  rdb,
	}
}

func (ps *pubSubRedis[T]) Publish(payload T) error {
	payloadEncoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}

	return ps.rdb.Publish(context.Background(),
		ps.name, string(payloadEncoded)).Err()
}

func (ps *pubSubRedis[T]) Subscribe() <-chan Result[T] {
	pubsub := ps.rdb.Subscribe(context.Background(), ps.name)
	ch := pubsub.Channel()

	out := make(chan Result[T], pubSubChanBufSize)

	go func() {
		for msg := range ch {
			payload := new(T)
			err := json.Unmarshal([]byte(msg.Payload), payload)
			out <- Result[T]{Ok: *payload, Err: err}
		}
	}()

	return out
}
