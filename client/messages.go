package client

import "github.com/tokenized/pkg/bitcoin"

type Message struct {
	Script bitcoin.Script
}

type Messages []*Message
