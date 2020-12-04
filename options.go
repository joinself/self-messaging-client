// Copyright 2020 Self Group Ltd. All Rights Reserved.

package messaging

import (
	"time"

	msgproto "github.com/selfid-net/self-messaging-client/proto"
)

// SendBuffer sets the size of the send buffer
func SendBuffer(sz int) func(c *Client) error {
	return func(c *Client) error {
		c.send = make(chan *request, sz)
		return nil
	}
}

// ReceiveBuffer sets the size of the receive buffer
func ReceiveBuffer(sz int) func(c *Client) error {
	return func(c *Client) error {
		c.recv = make(chan *msgproto.Message, sz)
		return nil
	}
}

// AutoReconnect enables retrying a connection if it closes unexpectedly
func AutoReconnect(enabled bool) func(c *Client) error {
	return func(c *Client) error {
		c.reconnect = true
		return nil
	}
}

// ReadDeadline sets the tcp read timeout
func ReadDeadline(deadline time.Duration) func(c *Client) error {
	return func(c *Client) error {
		c.deadline = deadline
		return nil
	}
}
