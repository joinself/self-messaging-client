package messaging

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	msgproto "github.com/selfid-net/self-messaging-proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ed25519"
	"gopkg.in/square/go-jose.v2"
)

var token string
var privkey string
var pubkey ed25519.PublicKey

func init() {
	token, privkey, pubkey = testToken("test")
}

func wait(ch chan msgproto.Message) (*msgproto.Message, error) {
	select {
	case msg := <-ch:
		return &msg, nil
	case <-time.After(time.Millisecond * 100):
		return nil, errors.New("channel read timeout")
	}
}

func testToken(id string) (string, string, ed25519.PublicKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)

	claims, err := json.Marshal(map[string]interface{}{
		"jti": uuid.New().String(),
		"iss": id,
		"exp": TimeFunc().Add(time.Minute).Unix(),
	})

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: priv}, nil)
	if err != nil {
		panic(err)
	}

	signedPayload, err := signer.Sign(claims)
	if err != nil {
		panic(err)
	}

	token, err := signedPayload.CompactSerialize()
	if err != nil {
		panic(err)
	}

	return token, base64.RawStdEncoding.EncodeToString(priv.Seed()), pub
}

func TestClient(t *testing.T) {
	s := newServer()
	defer s.close()

	c, err := New(s.endpoint, "someID", "1", privkey)
	require.Nil(t, err)
	require.NotNil(t, c)
}

func TestClientSend(t *testing.T) {
	s := newServer()
	defer s.close()

	c, err := New(s.endpoint, "someID", "1", privkey)
	require.Nil(t, err)
	require.NotNil(t, c)

	m := &msgproto.Message{Type: msgproto.MsgType_MSG, Sender: "test", Recipient: "tset", Ciphertext: []byte("hello")}
	err = c.Send(m)
	require.Nil(t, err)

	rm, err := wait(s.in)
	require.Nil(t, err)

	assert.Equal(t, msgproto.MsgType_MSG, rm.Type)
	assert.Equal(t, "test", rm.Sender)
	assert.Equal(t, "tset", rm.Recipient)
	assert.Equal(t, []byte("hello"), rm.Ciphertext)
}

func TestClientReceive(t *testing.T) {
	s := newServer()
	defer s.close()

	c, err := New(s.endpoint, "someID", "1", privkey)
	require.Nil(t, err)
	require.NotNil(t, c)

	s.out <- &msgproto.Message{Type: msgproto.MsgType_MSG, Sender: "test", Recipient: "tset", Ciphertext: []byte("hello")}

	m, err := c.Receive()
	require.Nil(t, err)
	require.NotNil(t, m)

	assert.Equal(t, msgproto.MsgType_MSG, m.Type)
	assert.Equal(t, "test", m.Sender)
	assert.Equal(t, "tset", m.Recipient)
	assert.Equal(t, []byte("hello"), m.Ciphertext)
}

func TestClientBusy(t *testing.T) {
	s := newServer()

	for i := 0; i < 1024; i++ {
		s.out <- &msgproto.Message{Type: msgproto.MsgType_MSG, Sender: "test", Recipient: "tset", Ciphertext: []byte("hello")}
	}

	c, err := New(s.endpoint, "someID", "1", privkey)
	require.Nil(t, err)

	go func() {
		for {
			_, cerr := c.Receive()
			require.Nil(t, cerr)
		}
	}()

	err = c.PermitAll()
	require.Nil(t, err)
}
