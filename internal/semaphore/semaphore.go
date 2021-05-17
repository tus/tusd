// Package semaphore implements a basic semaphore for coordinating and limiting
// non-exclusive, concurrent access.
package semaphore

type Semaphore chan struct{}

// New creates a semaphore with the given concurrency limit.
func New(concurrency int) Semaphore {
	return make(chan struct{}, concurrency)
}

// Acquire will block until the semaphore can be acquired.
func (s Semaphore) Acquire() {
	s <- struct{}{}
}

// Release frees the acquired slot in the semaphore.
func (s Semaphore) Release() {
	<-s
}
