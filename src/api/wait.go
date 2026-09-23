package api

import "sync"

var (
	requestWaitMu sync.Mutex
	requestWait   func(bool)
)

// SetRequestWait registers a callback around each live HTTP round trip.
// start is true when the request begins and false when it finishes (including retries).
// Pass nil to disable. The CLI uses this for a TTY spinner.
func SetRequestWait(fn func(start bool)) {
	requestWaitMu.Lock()
	requestWait = fn
	requestWaitMu.Unlock()
}

func beginRequestWait() {
	requestWaitMu.Lock()
	fn := requestWait
	requestWaitMu.Unlock()
	if fn != nil {
		fn(true)
	}
}

func endRequestWait() {
	requestWaitMu.Lock()
	fn := requestWait
	requestWaitMu.Unlock()
	if fn != nil {
		fn(false)
	}
}
