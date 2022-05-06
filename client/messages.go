package client

import (
	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"
)

type Message struct {
	Payload            bitcoin.Script      `bsor:"2" json:"payload"`
	Received           *channels.Timestamp `bsor:"3" json:"received,omitempty"`
	Sent               *channels.Timestamp `bsor:"4" json:"sent,omitempty"`
	IsAwaitingResponse bool                `bsor:"5" json:"is_awaiting_response,omitempty"`
	IsProcessed        bool                `bsor:"6" json:"is_processed,omitempty"`
}

type Messages []*Message

type ChannelMessage struct {
	Message Message
	Channel *Channel
}

type ChannelMessages []*ChannelMessage
