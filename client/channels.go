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

type Channel struct {
	Channel channels.Channel
	Entity  channels.Entity

	Messages   Messages
	MessageMap map[bitcoin.Hash32]int

	sync.RWMutex
}

type Channels []*Channel

func NewChannel(entity channels.Entity) *Channel {
	return &Channel{
		Entity:     entity,
		MessageMap: make(map[bitcoin.Hash32]int),
	}
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

	if !bytes.Equal(protocolIDs[0], channels.ProtocolIDSignedMessages) {
		return ErrSignatureRequired
	}

	var signature *channels.Signature
	signature, protocolIDs, payload, err = channels.ParseSigned(protocolIDs, payload)
	if err != nil {
		return errors.Wrap(err, "signed")
	}

	isInitial := false
	if c.Channel.Received == nil {
		isInitial = true
		if !bytes.Equal(protocolIDs[0], channels.ProtocolIDRelationships) {
			return ErrMissingRelationship
		}

		relationship, err := channels.ParseRelationship(protocolIDs, payload)
		if err != nil {
			return errors.Wrap(err, "relationship")
		}

		switch rm := relationship.(type) {
		case channels.RelationshipInitiation:
			relationship := channels.Entity(rm)
			c.Channel.Received = &relationship
		case channels.RelationshipAccept:
			relationship := channels.Entity(rm)
			c.Channel.Received = &relationship
		}

		if c.Channel.Sent == nil {
			// Send entity in accept
		}
	}

	if signature.PublicKey != nil &&
		!signature.PublicKey.Equal(c.Channel.Received.Identity.PublicKey) {
		return ErrWrongPublicKey
	}

	signature.SetPublicKey(&c.Channel.Received.Identity.PublicKey)
	if err := signature.Verify(); err != nil {
		return errors.Wrap(err, "verify signature")
	}

	if !isInitial {
		// if err := c.handleMessage(ctx, protocolIDs, payload); err != nil {
		// 	return errors.Wrap(err, "handle")
		// }
	}

	c.Messages = append(c.Messages, &Message{
		Script: msg,
	})
	c.MessageMap[*hash] = len(c.Messages)
	return nil
}
