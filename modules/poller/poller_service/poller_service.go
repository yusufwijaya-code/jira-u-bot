package poller_service

import "context"

type PollerService interface {
	// Run blocks until ctx is cancelled, polling every 30 seconds. Callers
	// launch it as a goroutine (see main.go, Task 1d.2) — it never returns
	// until shutdown.
	Run(ctx context.Context)
}
