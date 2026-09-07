package configuration

import (
	"testing"
	"time"
)

func TestShutdownSignalClosesOnceAndRemainsObservable(t *testing.T) {
	signal := NewShutdownSignal()
	if !signal.RequestShutdown() {
		t.Fatal("expected first shutdown request to be accepted")
	}
	if signal.RequestShutdown() {
		t.Fatal("expected repeated shutdown request to be idempotent")
	}
	select {
	case <-signal.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown signal was not closed")
	}
}
