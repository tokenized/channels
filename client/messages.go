package client

import (
	"github.com/tokenized/channels"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
)

type Message struct {
	ProtocolIDs        envelope.ProtocolIDs `bsor:"1" json:"protocol_ids"`
	Payload            bitcoin.ScriptItems  `bsor:"2" json:"payload"`
	Received           *channels.Timestamp  `bsor:"3" json:"received,omitempty"`
	Sent               *channels.Timestamp  `bsor:"4" json:"sent,omitempty"`
	IsAwaitingResponse bool                 `bsor:"5" json:"is_awaiting_response,omitempty"`
	IsProcessed        bool                 `bsor:"6" json:"is_processed,omitempty"`
}

type Messages []*Message

type ChannelMessage struct {
	Message Message
	Channel *Channel
}

type ChannelMessages []*ChannelMessage
