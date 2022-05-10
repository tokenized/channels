package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/tokenized/channels"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"

	"github.com/pkg/errors"
)

var (
	ErrAlreadyEstablished = errors.New("Already Established")
	ErrIsPublic           = errors.New("Is Public")
)

type Channel struct {
	// isPublic means this channel is publicly writable. Other users can send new relationship
	// initiation messages to it and will be directed to new channels if they are accepted.
	// Other communication like one time requests might also be available through these.
	isPublic bool

	// Hash used to derive channel's base key
	hash bitcoin.Hash32

	externalPublicKey *bitcoin.PublicKey

	// internal represents the local identity and the peer channels that messages are received on
	// and those messages.
	internal *CommunicationChannel

	// external represents the other identity and the peers channels that messages are sent to and
	// those messages.
	external *CommunicationChannel

	peerChannelsFactory *peer_channels.Factory

	lock sync.RWMutex
}

type Channels []*Channel

func NewPrivateChannel(peerChannelsFactory *peer_channels.Factory, hash bitcoin.Hash32,
	internalPeerChannels channels.PeerChannels) *Channel {
	return &Channel{
		hash:                hash,
		internal:            NewCommunicationChannel(internalPeerChannels),
		external:            NewCommunicationChannel(nil),
		peerChannelsFactory: peerChannelsFactory,
	}
}

func NewPublicChannel(peerChannelsFactory *peer_channels.Factory, hash bitcoin.Hash32,
	internalPeerChannels channels.PeerChannels) *Channel {
	return &Channel{
		isPublic:            true,
		hash:                hash,
		internal:            NewCommunicationChannel(internalPeerChannels),
		external:            NewCommunicationChannel(nil),
		peerChannelsFactory: peerChannelsFactory,
	}
}

func (c *Channel) IsPublic() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.isPublic
}

func (c *Channel) Hash() bitcoin.Hash32 {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.hash
}

func (c *Channel) IncomingPeerChannels() channels.PeerChannels {
	return c.internal.PeerChannels()
}

func (c *Channel) SetExternalPeerChannels(peerChannels channels.PeerChannels) error {
	return c.external.SetPeerChannels(peerChannels)
}

func (c *Channel) Initialize(ctx context.Context,
	initiation *channels.RelationshipInitiation) error {

	if c.IsPublic() {
		return ErrIsPublic
	}

	if err := c.external.SetPeerChannels(initiation.PeerChannels); err != nil {
		return errors.Wrap(err, "set entity")
	}

	if err := c.SetExternalPublicKey(initiation.PublicKey); err != nil {
		return errors.Wrap(err, "set external public key")
	}

	return nil
}

func (c *Channel) SendMessage(ctx context.Context, message bitcoin.Script) error {
	return c.external.sendMessage(ctx, c.peerChannelsFactory, message)
}

func (c *Channel) MarkMessageProcessed(ctx context.Context, hash bitcoin.Hash32) error {
	return c.internal.MarkMessageProcessed(ctx, hash)
}

func (c *Channel) Reject(ctx context.Context, messageHash bitcoin.Hash32,
	reject *channels.Reject) error {

	reject.MessageHash = messageHash

	logger.InfoWithFields(ctx, []logger.Field{
		logger.JSON("reject", reject),
		logger.String("reject_code", reject.CodeToString()),
	}, "Adding reject")

	payload, err := reject.Write()
	if err != nil {
		return errors.Wrap(err, "write")
	}

	scriptItems := envelopeV1.Wrap(payload)
	script, err := scriptItems.Script()
	if err != nil {
		return errors.Wrap(err, "script")
	}

	// TODO Figure out how this will get signed. --ce
	return c.external.addMessage(ctx, script)
}

func (c *Channel) GetExternalPublicKey() *bitcoin.PublicKey {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.externalPublicKey
}

func (c *Channel) SetExternalPublicKey(publicKey bitcoin.PublicKey) error {
	c.lock.RLock()
	defer c.lock.RUnlock()

	c.externalPublicKey = &publicKey
	return nil
}

func (c *Channel) ProcessMessage(ctx context.Context, message *peer_channels.Message) error {
	if err := c.internal.addMessage(ctx, bitcoin.Script(message.Payload)); err != nil {
		return errors.Wrap(err, "add message")
	}

	wMessage, err := channels.Unwrap(bitcoin.Script(message.Payload))
	if err != nil {
		return errors.Wrap(err, "unwrap")
	}

	if wMessage.Signature == nil {
		if err := c.Reject(ctx, message.Hash(), &channels.Reject{
			Reason:           channels.RejectReasonInvalid,
			RejectProtocolID: channels.ProtocolIDSignedMessages,
			Code:             channels.SignedRejectCodeSignatureRequired,
		}); err != nil {
			return errors.Wrap(err, "no signature: reject")
		}
		return nil
	}

	if wMessage.Response != nil {
		logger.InfoWithFields(ctx, []logger.Field{
			logger.Stringer("response_hash", wMessage.Response.MessageHash),
		}, "Response")
	}

	publicKey := c.GetExternalPublicKey()

	var entity *channels.Entity
	switch msg := wMessage.Message.(type) {
	case *channels.RelationshipInitiation:
		fmt.Printf("Is Relationship\n")
		relationship := channels.Entity(*msg)
		entity = &relationship
		if publicKey == nil {
			// Use newly established relationship key
			publicKey = &entity.PublicKey
		}
	}

	if publicKey == nil {
		if err := c.Reject(ctx, message.Hash(), &channels.Reject{
			Reason:           channels.RejectReasonInvalid,
			RejectProtocolID: channels.ProtocolIDRelationships,
			Code:             channels.RelationshipsRejectCodeNotInitiated,
		}); err != nil {
			return errors.Wrap(err, "no relationship: reject")
		}
		return nil
	}

	if wMessage.Signature.PublicKey != nil {
		if !wMessage.Signature.PublicKey.Equal(*publicKey) {
			return ErrWrongPublicKey
		}
	} else {
		wMessage.Signature.SetPublicKey(publicKey)
	}

	if err := wMessage.Signature.Verify(); err != nil {
		var code uint32
		if errors.Cause(err) == channels.ErrInvalidSignature {
			code = channels.SignedRejectCodeInvalidSignature
		}
		if err := c.Reject(ctx, message.Hash(), &channels.Reject{
			Reason:           channels.RejectReasonInvalid,
			RejectProtocolID: channels.ProtocolIDSignedMessages,
			Code:             code,
		}); err != nil {
			return errors.Wrap(err, "reject")
		}
		return nil
	}

	if !c.IsPublic() && entity != nil {
		if err := c.external.SetPeerChannels(entity.PeerChannels); err != nil {
			// TODO Allow entity updates --ce
			if errors.Cause(err) == ErrAlreadyEstablished {
				fmt.Printf("Already have entity\n")
				if err := c.Reject(ctx, message.Hash(), &channels.Reject{
					Reason:           channels.RejectReasonInvalid,
					RejectProtocolID: channels.ProtocolIDRelationships,
					Code:             channels.RelationshipsRejectCodeAlreadyInitiated,
				}); err != nil {
					return errors.Wrap(err, "already have entity: reject")
				}
			} else {
				return errors.Wrap(err, "set entity")
			}
		}

		if err := c.SetExternalPublicKey(entity.PublicKey); err != nil {
			return errors.Wrap(err, "set external public key")
		}
	}

	return nil
}
