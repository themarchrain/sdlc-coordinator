package configuration

import "sync"

type ShutdownRequester interface {
	RequestShutdown() bool
}

type ShutdownSignal struct {
	once sync.Once
	done chan struct{}
}

func NewShutdownSignal() *ShutdownSignal {
	return &ShutdownSignal{done: make(chan struct{})}
}

func (signal *ShutdownSignal) RequestShutdown() bool {
	requested := false
	signal.once.Do(func() {
		requested = true
		close(signal.done)
	})
	return requested
}

func (signal *ShutdownSignal) Done() <-chan struct{} {
	return signal.done
}
