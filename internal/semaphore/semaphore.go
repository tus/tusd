package semaphore

type Semaphore chan struct{}

func New(concurrency int) Semaphore {
	return make(chan struct{}, concurrency)
}

func (s Semaphore) Acquire() {
	s <- struct{}{}
}

func (s Semaphore) Release() {
	<-s
}
