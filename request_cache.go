package messaging

import (
	"errors"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	msgproto "github.com/selfid-net/self-messaging-proto"
)

// requestCache stores requests that expect a response from the server
type requestCache struct {
	requests    map[string]chan proto.Message
	jwsRequests map[string]chan *msgproto.Message
	mu          sync.RWMutex
	jwsmu       sync.RWMutex
}

func newRequestCache() *requestCache {
	return &requestCache{
		requests:    make(map[string]chan proto.Message),
		jwsRequests: make(map[string]chan *msgproto.Message),
	}
}

// Send sends a response to the waiting thread
func (rc *requestCache) send(reqID string, m proto.Message) {
	rc.mu.Lock()
	ch, ok := rc.requests[reqID]
	rc.mu.Unlock()

	if ok {
		ch <- m
	}
}

// Register makes a request
func (rc *requestCache) register(reqID string) chan proto.Message {
	ch := make(chan proto.Message, 1)

	rc.mu.Lock()
	rc.requests[reqID] = ch
	rc.mu.Unlock()

	return ch
}

// Cancel cancels a request
func (rc *requestCache) cancel(reqID string) {
	rc.mu.Lock()
	delete(rc.requests, reqID)
	rc.mu.Unlock()
}

// Wait for a response from the server
func (rc *requestCache) wait(reqID string, timeout time.Duration) (proto.Message, error) {
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

// Send sends a response to the waiting thread. Will return true if there is a valid request registered
func (rc *requestCache) sendJWS(reqID string, m *msgproto.Message) bool {
	rc.jwsmu.Lock()
	ch, ok := rc.jwsRequests[reqID]
	rc.jwsmu.Unlock()

	if !ok {
		return false
	}

	ch <- m

	return false
}

// Register makes a request
func (rc *requestCache) registerJWS(reqID string) chan *msgproto.Message {
	ch := make(chan *msgproto.Message, 1)

	rc.jwsmu.Lock()
	rc.jwsRequests[reqID] = ch
	rc.jwsmu.Unlock()

	return ch
}

// Cancel cancels a request
func (rc *requestCache) cancelJWS(reqID string) {
	rc.jwsmu.Lock()
	delete(rc.jwsRequests, reqID)
	rc.jwsmu.Unlock()
}

// Wait for a response from the server
func (rc *requestCache) waitJWS(reqID string, timeout time.Duration) (*msgproto.Message, error) {
	rc.jwsmu.RLock()
	ch := rc.jwsRequests[reqID]
	rc.jwsmu.RUnlock()

	defer func() {
		rc.jwsmu.Lock()
		delete(rc.jwsRequests, reqID)
		rc.jwsmu.Unlock()
	}()

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(timeout):
		return nil, errors.New("request timed out")
	}
}
