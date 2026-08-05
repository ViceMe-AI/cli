package processlock

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWithSerializesIndependentLockInstances(t *testing.T) {
	root := t.TempDir()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondEntered := make(chan struct{})
	var active atomic.Int32
	results := make(chan error, 2)

	go func() {
		results <- With(context.Background(), root, "credential", "same-key", func() error {
			if active.Add(1) != 1 {
				t.Error("first action overlapped another lock holder")
			}
			close(firstStarted)
			<-releaseFirst
			active.Add(-1)
			return nil
		})
	}()
	<-firstStarted
	go func() {
		results <- With(context.Background(), root, "credential", "same-key", func() error {
			if active.Add(1) != 1 {
				t.Error("second action overlapped another lock holder")
			}
			close(secondEntered)
			active.Add(-1)
			return nil
		})
	}()

	select {
	case <-secondEntered:
		t.Fatal("second action entered before the first released the lock")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}
