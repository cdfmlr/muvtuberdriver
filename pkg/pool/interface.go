// Package pool provide a generic connection pool.
package pool

import (
	"errors"
	"io"
)

type Poolable interface {
	io.Closer
}

type Pool[T Poolable] interface {
	Get() (T, error) // Get a T from the pool
	Put(T) error     // Put T back
	Release(T) error // Release unused T from pool, and Close it. Release a T that is not Get from Pool breaks the consistency! 
	Len() int        // Len is current pool size
	Close() error    // Close the pool: Refuse to Get, Release all holding/Putting-back resources.
}

// errors
var (
	ErrPoolClosed    = errors.New("the pool is closed")
	ErrPoolExhausted = errors.New("the pool has been exhausted")
)
