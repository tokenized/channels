package client

import (
	"bytes"
	"context"
	"sync"

	"github.com/tokenized/channels"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

var (
	ErrNotSigned = errors.New("Not Signed")
)

type Client struct {
	Channels Channels

	sync.RWMutex
}

type Channel struct {
	Channel channels.Channel

	Messages   Messages
	MessageMap map[bitcoin.Hash32]int

	sync.RWMutex
}

type Channels []*Channel

func (c *Client) Listen(ctx context.Context, interrupt <-chan interface{}) error {

}

func (c *Client) AddChannel(ctx context.Context, channel *Channel) error {
	c.Lock()
	defer c.Unlock()

}

func (c *Channel) ProcessMessage(ctx context.Context, msg bitcoin.Script) error {
	c.Lock()
	defer c.Unlock()

	hash, _ := bitcoin.NewHash32(bitcoin.DoubleSha256(msg))
	if _, exists := c.MessageMap[*hash]; exists {
		return nil
	}

	protocolIDs, payload, err := envelopeV1.Parse(bytes.NewReader(msg))
	if err != nil {
		return errors.Wrap(err, "envelope")
	}

	if !bytes.Equal(ProtocolIDSignedMessages) {
		return ErrNotSigned
	}

	var signature *channels.Signature
	signature, protocolIDs, payload, err = ParseSigned(protocolIDs, payload)
	if err != nil {
		return errors.Wrap(err, "signed")
	}

	if err := c.handleMessage(ctx, protocolIDs, payload); err != nil {
		return errors.Wrap(err, "handle")
	}

	if err := signature.Verify(); err != nil {
		return errors.Wrap(err, "verify signature")
	}

	c.MessageMap[*hash] = msg
	return nil
}
