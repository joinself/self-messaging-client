package messaging

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/websocket"
	msgproto "github.com/selfid-net/self-messaging-proto"
	"gopkg.in/square/go-jose.v2"
)

type testserver struct {
	s        *httptest.Server
	in       chan msgproto.Message
	out      chan interface{}
	endpoint string
}

func newServer() *testserver {
	s := testserver{in: make(chan msgproto.Message), out: make(chan interface{}, 1024)}
	m := http.NewServeMux()
	m.HandleFunc("/", s.testHandler)
	s.s = httptest.NewServer(m)
	s.endpoint = "ws" + strings.TrimPrefix(s.s.URL, "http")
	return &s
}

func errorMessage(id string, err error) []byte {
	m, err := proto.Marshal(&msgproto.Notification{Type: msgproto.MsgType_ERR, Id: id, Error: err.Error()})
	if err != nil {
		log.Println(err)
	}
	return m
}

func (t *testserver) testHandler(w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{}

	wc, err := u.Upgrade(w, r, nil)
	if err != nil {
		panic(err)
	}

	_, msg, err := wc.ReadMessage()
	if err != nil {
		panic(err)
	}

	var req msgproto.Auth

	err = proto.Unmarshal(msg, &req)
	if err != nil {
		wc.WriteMessage(websocket.BinaryMessage, errorMessage(req.Id, err))
		panic(err)
	}

	rt, err := jose.ParseSigned(req.Token)
	if err != nil {
		panic(err)
	}

	payload, err := rt.Verify(pubkey)
	if err != nil {
		wc.WriteMessage(websocket.BinaryMessage, errorMessage(req.Id, err))
		panic(err)
	}

	var claims map[string]interface{}

	err = json.Unmarshal(payload, &claims)
	if err != nil {
		wc.WriteMessage(websocket.BinaryMessage, errorMessage(req.Id, err))
		panic(err)
	}

	_, ok := claims["iss"]
	if !ok {
		wc.WriteMessage(websocket.BinaryMessage, errorMessage(req.Id, errors.New("invalid issuer")))
		panic("invalid issuer")
	}

	data, _ := proto.Marshal(&msgproto.Notification{Type: msgproto.MsgType_ACK, Id: req.Id})
	wc.WriteMessage(websocket.BinaryMessage, data)

	go func() {
		for {
			var h msgproto.Header

			_, data, err := wc.ReadMessage()
			if err != nil {
				return
			}

			err = proto.Unmarshal(data, &h)
			if err != nil {
				log.Println(err)
				return
			}

			t.out <- &msgproto.Notification{Type: msgproto.MsgType_ACK, Id: h.Id}

			if h.Type == msgproto.MsgType_MSG {
				var m msgproto.Message

				err = proto.Unmarshal(data, &m)
				if err != nil {
					log.Println(err)
					return
				}

				t.in <- m
			}
		}
	}()

	go func() {
		for {
			var data []byte
			var err error

			e := <-t.out

			switch v := e.(type) {
			case *msgproto.Message:
				data, err = proto.Marshal(v)
			case *msgproto.Notification:
				data, err = proto.Marshal(v)
			}

			if err != nil {
				log.Println(err)
				return
			}

			wc.WriteMessage(websocket.BinaryMessage, data)
		}
	}()
}

func (t *testserver) close() {
	t.s.Close()
}
