# self messaging client [![GoDoc](https://godoc.org/github.com/selfid-net/self-messaging-client?status.svg)](https://godoc.org/github.com/selfid-net/self-messaging-client) [![Go Report Card](https://goreportcard.com/badge/github.com/selfid-net/self-messaging-client)](https://goreportcard.com/report/github.com/selfid-net/self-messaging-client) [![Build Status](https://travis-ci.com/selfid-net/self-messaging-client.svg?branch=master)](https://travis-ci.com/selfid-net/self-messaging-client)

A messaging client for go for use with self's messaging service

# Installation

To start using this client, you can run:

`$ go get github.com/selfid-net/self-messaging-client`

# Usage

To create a new messaging client

```go
package main

import (
    messaging "github.com/selfid-net/self-messaging-client"
    msgproto "github.com/selfid-net/self-messaging-client/proto"
)

func main() {
    appID := "5d41402abc4b2a76b9719d911017c592" // the ID of your application
    device := "1"                               // the device identifier you are connecting as
    appKey := "secret-key"                      // the applications private key

    // create a new messaging client
    client, err := messaging.New("wss://messaging.selfid.net", appID, device, appKey)
}
```


Additional optional parameters can be specified to the client like:

```go
func main() {
    ...

    client, err := messaging.New("wss://messaging.selfid.net", appID, device, appKey, messaging.AutoReconnect(true))
}
```

You can send a message by using the following:

```go
func main() {
    ...

    msg := &msgproto.Message{
        Id: uuid.New().String(),                      // the id of the request
        Type: msgproto.MsgType_MSG,                   // the type of message
        Sender: "5d41402abc4b2a76b9719d911017c592:1", // constructed from app ID and device
        Recipient: "12345678910:aeH2o21",             // the recipients self ID and device
        Ciphertext: []byte("hello"),                  // the messages payload
    }

    err = client.Send(msg)
}
```

There are two ways to receive a message:

```go
func main() {
    ...

    // read message
    for {
        msg, err := client.Receive()
    }

    // or read via channel
    mch := client.ReceiveChan()
    for {
        if !client.IsClosed() {
            return
        }

        msg := <-mch
    }
}
```


## Versioning

For transparency into our release cycle and in striving to maintain backward
compatibility, this project is maintained under [the Semantic Versioning guidelines](http://semver.org/).

## Copyright and License

Code and documentation copyright since 2019 Self ID LTD.

Code released under
[the MIT License](LICENSE).
