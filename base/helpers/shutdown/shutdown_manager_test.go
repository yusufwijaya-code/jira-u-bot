package shutdown

import (
	"testing"
	"time"
)

func TestManager_ShutdownCancelsContext(t *testing.T) {
	m := NewManager()

	select {
	case <-m.Context().Done():
		t.Fatal("context should not be done before Shutdown() is called")
	default:
	}

	m.Shutdown()

	select {
	case <-m.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("context should be done immediately after Shutdown()")
	}
}

func TestManager_WaitBlocksUntilAllWorkersDone(t *testing.T) {
	m := NewManager()
	m.AddWorker()
	m.AddWorker()

	done := make(chan struct{})
	go func() {
		m.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Wait() returned before any worker called WorkerDone()")
	case <-time.After(50 * time.Millisecond):
	}

	m.WorkerDone()
	m.WorkerDone()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Wait() did not return after all workers called WorkerDone()")
	}
}
