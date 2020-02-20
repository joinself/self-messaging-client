package messaging

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	msgproto "github.com/selfid-net/self-messaging-proto"
	"golang.org/x/crypto/ed25519"
	"gopkg.in/square/go-jose.v2"
)

const (
	DefaultBufferSize = 128
	DefaultTimeout    = time.Second * 10
	DefaultDeadline   = time.Second * 10
	DefaultRetries    = 30
)

var (
	CloseMessage = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	TimeFunc     = NewTime().Now
)

type request struct {
	id       string
	message  []byte
	response chan error
}

// Client connection for self messaging
type Client struct {
	endpoint    string
	token       string
	selfID      string
	deviceID    string
	privateKey  string
	reconnect   bool
	maxretries  int
	deadline    time.Duration
	timeout     time.Duration
	ws          *websocket.Conn
	send        chan *request
	recv        chan *msgproto.Message
	closewriter chan bool
	requests    *requestCache
	closed      int32
}

// New create a new messaging client
func New(endpoint, selfID, deviceID, privateKey string, opts ...func(*Client) error) (*Client, error) {
	c := Client{
		endpoint:    endpoint,
		selfID:      selfID,
		deviceID:    deviceID,
		privateKey:  privateKey,
		timeout:     DefaultTimeout,
		deadline:    DefaultDeadline,
		maxretries:  DefaultRetries,
		send:        make(chan *request, DefaultBufferSize),
		recv:        make(chan *msgproto.Message, DefaultBufferSize),
		closewriter: make(chan bool),
		requests:    newRequestCache(),
	}

	for _, opt := range opts {
		err := opt(&c)
		if err != nil {
			return nil, err
		}
	}

	return &c, c.setup()
}

func (c *Client) setup() error {
	atomic.StoreInt32(&c.closed, 0)

	err := c.generateToken()
	if err != nil {
		return err
	}

	err = c.connect()
	if err != nil {
		return err
	}

	err = c.authenticate()
	if err != nil {
		return err
	}

	go c.reader()
	go c.writer()

	return nil
}

func (c *Client) tryReconnect(err error) {
	if !c.reconnect {
		return
	}

	switch e := err.(type) {
	case net.Error:
		if !e.Timeout() {
			return
		}
	case *websocket.CloseError:
		if e.Code != websocket.CloseAbnormalClosure {
			return
		}
	default:
		log.Println("unknown error type")
		spew.Dump(e)
	}

	for i := 0; i < c.maxretries; i++ {
		log.Println("attempting reconnect")

		err := c.setup()
		if err == nil {
			atomic.StoreInt32(&c.closed, 0)
			return
		}

		time.Sleep(DefaultTimeout)
	}
}

func (c *Client) generateToken() error {
	pks, _ := base64.RawStdEncoding.DecodeString(c.privateKey)
	pk := ed25519.NewKeyFromSeed(pks)

	claims, err := json.Marshal(map[string]interface{}{
		"jti": uuid.New().String(),
		"iss": c.selfID,
		"exp": TimeFunc().Add(time.Minute).Unix(),
	})

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: pk}, nil)
	if err != nil {
		return err
	}

	signedPayload, err := signer.Sign(claims)
	if err != nil {
		return err
	}

	token, err := signedPayload.CompactSerialize()
	if err != nil {
		return err
	}

	c.token = string(token)

	return nil
}

func (c *Client) connect() error {
	ws, _, err := websocket.DefaultDialer.Dial(c.endpoint, nil)
	if err != nil {
		return err
	}

	c.ws = ws

	ws.SetReadDeadline(time.Now().Add(c.deadline))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(c.deadline)); return nil })

	return nil
}

func (c *Client) authenticate() error {
	var resp msgproto.Notification

	auth := msgproto.Auth{
		Id:     uuid.New().String(),
		Type:   msgproto.MsgType_AUTH,
		Token:  c.token,
		Device: c.deviceID,
	}

	data, err := proto.Marshal(&auth)
	if err != nil {
		return err
	}

	err = c.ws.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		return err
	}

	_, data, err = c.ws.ReadMessage()
	if err != nil {
		return err
	}

	err = proto.Unmarshal(data, &resp)
	if err != nil {
		return err
	}

	switch resp.Type {
	case msgproto.MsgType_ACK:
		return nil
	case msgproto.MsgType_ERR:
		return errors.New(resp.Error)
	default:
		return errors.New("unknown authentication error")
	}
}

func (c *Client) reader() {
	for {
		if c.IsClosed() {
			return
		}

		_, data, err := c.ws.ReadMessage()
		if err != nil {
			c.close()
			c.tryReconnect(err)
			return
		}

		var hdr msgproto.Header

		err = proto.Unmarshal(data, &hdr)
		if err != nil {
			continue
		}

		var m proto.Message

		switch hdr.Type {
		case msgproto.MsgType_MSG:
			m = &msgproto.Message{}
		case msgproto.MsgType_ACL:
			m = &msgproto.AccessControlList{}
		case msgproto.MsgType_ACK, msgproto.MsgType_ERR:
			m = &msgproto.Notification{}
		}

		err = proto.Unmarshal(data, m)
		if err != nil {
			continue
		}

		switch hdr.Type {
		case msgproto.MsgType_ACK, msgproto.MsgType_ERR, msgproto.MsgType_ACL:
			c.requests.send(hdr.Id, m)
		case msgproto.MsgType_MSG:
			msg := m.(*msgproto.Message)
			msgID := getJWSResponseID(msg.Ciphertext)
			ok := c.requests.sendJWS(msgID, msg)
			if !ok {
				c.recv <- msg
			}
		}
	}
}

func (c *Client) writer() {
	var err error

	for {
		if c.IsClosed() {
			return
		}

		select {
		case <-c.closewriter:
			err = c.ws.WriteControl(websocket.CloseMessage, CloseMessage, time.Now().Add(c.deadline))
		case request := <-c.send:
			err = c.ws.WriteMessage(websocket.BinaryMessage, request.message)
			request.response <- err
		case <-time.After(c.deadline / 2):
			err = c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(c.deadline))
		}

		if err != nil {
			c.close()
			return
		}
	}
}

// Send send a message
func (c *Client) Send(m *msgproto.Message) error {
	if c.IsClosed() {
		return errors.New("connection is closed")
	}

	resp, err := c.request(m.Id, m)
	if err != nil {
		return err
	}

	n, ok := resp.(*msgproto.Notification)
	if ok {
		if n.Type == msgproto.MsgType_ACK {
			return nil
		}
		if n.Type == msgproto.MsgType_ERR {
			return errors.New(n.Error)
		}
	}

	return nil
}

// Receive receive a message
func (c *Client) Receive() (*msgproto.Message, error) {
	for {
		select {
		case m := <-c.recv:
			return m, nil
		case <-time.After(time.Second):
			if c.IsClosed() {
				return nil, errors.New("connection is closed")
			}
		}
	}
}

// ReceiveChan returns a channel of all incoming messages
func (c *Client) ReceiveChan() chan *msgproto.Message {
	return c.recv
}

// PermitAll permits messages from all identities
func (c *Client) PermitAll() error {
	return c.acl(msgproto.ACLCommand_PERMIT, "*", nil)
}

// PermitSender permits messages from a given sender
func (c *Client) PermitSender(selfID string, exp time.Time) error {
	return c.acl(msgproto.ACLCommand_PERMIT, selfID, &exp)
}

// BlockSender blocks messages from a given sender
func (c *Client) BlockSender(selfID string) error {
	return c.acl(msgproto.ACLCommand_REVOKE, selfID, nil)
}

// ListACLRules returns all active ACL rules for the authenticated identity
func (c *Client) ListACLRules() ([]ACLRule, error) {
	var rules []ACLRule

	req := msgproto.AccessControlList{
		Id:      uuid.New().String(),
		Type:    msgproto.MsgType_ACL,
		Command: msgproto.ACLCommand_LIST,
	}

	resp, err := c.request(req.Id, &req)
	if err != nil {
		return rules, err
	}

	switch r := resp.(type) {
	case *msgproto.Notification:
		err = errors.New(r.Error)
	case *msgproto.AccessControlList:
		err = json.Unmarshal(r.Payload, &rules)
	}

	return rules, err
}

// JWSRequest makes a JWS request and returns the response
func (c *Client) JWSRequest(id string, m *msgproto.Message) (chan *msgproto.Message, error) {
	ch := c.requests.registerJWS(id)

	err := c.Send(m)
	if err != nil {
		c.requests.cancelJWS(id)
		return nil, err
	}

	return ch, nil
}

// JWSResponse waits for a message response for a given JWS request
func (c *Client) JWSResponse(id string, timeout time.Duration) (*msgproto.Message, error) {
	return c.requests.waitJWS(id, timeout)
}

// JWSRegister registers a jws request by id
func (c *Client) JWSRegister(id string) {
	c.requests.registerJWS(id)
}

// Request send a message that expects a response
func (c *Client) request(id string, m proto.Message) (proto.Message, error) {
	if c.IsClosed() {
		return nil, errors.New("connection is closed")
	}

	data, err := proto.Marshal(m)
	if err != nil {
		return nil, err
	}

	r := request{id: id, message: data, response: make(chan error)}
	c.requests.register(r.id)
	c.send <- &r

	err = <-r.response
	if err != nil {
		return nil, err
	}

	resp, err := c.requests.wait(r.id, c.timeout)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *Client) acl(action msgproto.ACLCommand, selfID string, exp *time.Time) error {
	rule := map[string]string{
		"iss":        c.selfID,
		"exp":        time.Now().Add(time.Minute).Format(time.RFC3339),
		"jti":        uuid.New().String(),
		"acl_source": selfID,
	}

	if exp != nil {
		rule["acl_exp"] = exp.Format(time.RFC3339)
	}

	payload, err := json.Marshal(rule)
	if err != nil {
		return err
	}

	pks, _ := base64.RawStdEncoding.DecodeString(c.privateKey)
	pk := ed25519.NewKeyFromSeed(pks)

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: pk}, nil)
	if err != nil {
		return err
	}

	signedPayload, err := signer.Sign(payload)
	if err != nil {
		return err
	}

	acl := msgproto.AccessControlList{
		Id:      uuid.New().String(),
		Type:    msgproto.MsgType_ACL,
		Command: action,
		Payload: []byte(signedPayload.FullSerialize()),
	}

	resp, err := c.request(acl.Id, &acl)
	if err != nil {
		return err
	}

	n, ok := resp.(*msgproto.Notification)
	if !ok {
		return errors.New("received invalid response from server")
	}

	switch n.Type {
	case msgproto.MsgType_ACK:
		return nil
	case msgproto.MsgType_ERR:
		return errors.New(n.Error)
	default:
		return errors.New("unknown response from server")
	}
}

// IsClosed returns true if the connection is closed
func (c *Client) IsClosed() bool {
	return atomic.LoadInt32(&(c.closed)) != 0
}

func (c *Client) Close() {
	c.closewriter <- true
	time.Sleep(time.Millisecond * 10)
	c.ws.Close()
}

func (c *Client) close() {
	if c.IsClosed() {
		return
	}

	atomic.StoreInt32(&(c.closed), int32(1))

	c.Close()
}
