package modbus

import "sync/atomic"

type AtomicCounter struct {
	counter int32
}

func (a *AtomicCounter) Add(delta int32) {
	atomic.AddInt32(&a.counter, delta)
}

func (a *AtomicCounter) Load() int32 {
	return atomic.LoadInt32(&a.counter)
}
