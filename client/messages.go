package client

import (
	"crypto/sha256"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"
)

type Message struct {
	Payload            bitcoin.Script      `bsor:"1" json:"payload"`
	Received           *channels.Timestamp `bsor:"2" json:"received,omitempty"`
	Sent               *channels.Timestamp `bsor:"3" json:"sent,omitempty"`
	IsAwaitingResponse bool                `bsor:"4" json:"is_awaiting_response,omitempty"`
	IsProcessed        bool                `bsor:"5" json:"is_processed,omitempty"`
}

type Messages []*Message

type ChannelMessage struct {
	Message Message
	Channel *Channel
}

type ChannelMessages []*ChannelMessage

func (m Message) Hash() bitcoin.Hash32 {
	return bitcoin.Hash32(sha256.Sum256(m.Payload))
}
