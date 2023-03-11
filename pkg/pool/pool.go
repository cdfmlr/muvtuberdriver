// Package pool provide a generic connection pool.
package pool

import (
	"sync/atomic"
)

// pool is a naive & mutex-free Pool implementation.
//
// I can proof its correctness (function, safety, inevitability, fair).
type pool[T Poolable] struct {
	entries chan T
	create  func() (T, error)

	maxLen int64
	len    atomic.Int64 // len < 0: closed
}

func NewPool[T Poolable](maxLen int64, createT func() (T, error)) Pool[T] {
	return &pool[T]{
		entries: make(chan T, maxLen),
		create:  createT,
		maxLen:  maxLen,
	}
}

func (p *pool[T]) Get() (T, error) {
	if p.closed() {
		return *new(T), ErrPoolClosed
	}

	select {
	case t, ok := <-p.entries:
		if !ok { // never happen: the p.closed does promise.
			return *new(T), ErrPoolClosed
		}
		return t, nil
	default:
		if p.len.Add(1) >= p.maxLen {
			return *new(T), ErrPoolExhausted
		}
		t, err := p.create()
		if err != nil {
			p.len.Add(-1)
		}
		return t, err
	}
}

func (p *pool[T]) Put(t T) error {
	if p.closed() {
		return p.Close()
	}

	select {
	case p.entries <- t:
		return nil
	default: // full
		return p.Close()
	}
}

func (p *pool[T]) Release(t T) error {
	p.len.Add(-1)
	return t.Close()
}

func (p *pool[T]) Len() int {
	return int(p.len.Load())
}

func (p *pool[T]) Idle() int {
	return len(p.entries)
}

func (p *pool[T]) Close() error {
	p.close()

	for t := range p.entries {
		t.Close()
	}

	close(p.entries)

	return nil
}

func (p *pool[T]) close() {
	p.len.Store(-1)
}

func (p *pool[T]) closed() bool {
	return p.len.Load() < 0
}
