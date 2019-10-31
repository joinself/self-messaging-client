package messaging

import (
	"errors"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
)

// requestCache stores requests that expect a response from the server
type requestCache struct {
	requests map[string]chan proto.Message
	mu       sync.RWMutex
}

func newRequestCache() *requestCache {
	return &requestCache{requests: make(map[string]chan proto.Message)}
}

// Send sends a response to the waiting thread
func (rc *requestCache) Send(reqID string, m proto.Message) {
	rc.mu.Lock()

	ch, ok := rc.requests[reqID]
	if ok {
		ch <- m
	}

	rc.mu.Unlock()
}

// Register a request
func (rc *requestCache) Register(reqID string) {
	rc.mu.Lock()
	rc.requests[reqID] = make(chan proto.Message, 1)
	rc.mu.Unlock()
}

// Wait for a response from the server
func (rc *requestCache) Wait(reqID string, timeout time.Duration) (proto.Message, error) {
	rc.mu.RLock()
	ch := rc.requests[reqID]
	rc.mu.RUnlock()

	defer func() {
		rc.mu.Lock()
		delete(rc.requests, reqID)
		rc.mu.Unlock()
	}()

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(timeout):
		return nil, errors.New("request timed out")
	}
}
