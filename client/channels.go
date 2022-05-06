package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/tokenized/channels"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"

	"github.com/pkg/errors"
)

var (
	ErrAlreadyHaveEntity = errors.New("Already Have Entity")
)

type Channel struct {
	// IsInitiation means this channel is for new entities to send relationship initiation messages
	// to that will be directed to new channels if they are accepted.
	isInitiation bool

	// Hash used to derive channel's base key
	hash bitcoin.Hash32

	// Incoming represents the local identity and the peer channels that messages are received on
	// and those messages.
	Incoming *ChannelCommunication

	// Outgoing represents the other identity and the peers channels that messages are sent to and
	// those messages.
	Outgoing *ChannelCommunication

	lock sync.Mutex
}

type ChannelCommunication struct {
	Entity *channels.Entity

	Messages   Messages
	MessageMap map[bitcoin.Hash32]int

	lock sync.RWMutex
}

type Channels []*Channel

func SupportedProtocols() envelope.ProtocolIDs {
	return envelope.ProtocolIDs{
		channels.ProtocolIDResponse,
		channels.ProtocolIDReject,
		channels.ProtocolIDSignedMessages,
		channels.ProtocolIDRelationships,
		// channels.ProtocolIDMerkleProof,
		// channels.ProtocolIDInvoices,
		// channels.ProtocolIDPeerChannels,
	}
}

func NewPrivateChannel(hash bitcoin.Hash32, publicKey bitcoin.PublicKey,
	incomingPeerChannels channels.PeerChannels, identity channels.Identity) *Channel {

	return &Channel{
		hash:     hash,
		Incoming: NewChannelCommunication(publicKey, incomingPeerChannels, identity),
		Outgoing: NewEmptyChannelCommunication(),
	}
}

func NewInitiationChannel(incomingPeerChannels channels.PeerChannels) *Channel {
	return &Channel{
		isInitiation: true,
		Incoming:     NewInitiationChannelCommunication(incomingPeerChannels),
		Outgoing:     NewEmptyChannelCommunication(),
	}
}

func NewChannelCommunication(publicKey bitcoin.PublicKey,
	peerChannels channels.PeerChannels, identity channels.Identity) *ChannelCommunication {

	return &ChannelCommunication{
		Entity: &channels.Entity{
			PublicKey:          publicKey,
			PeerChannels:       peerChannels,
			SupportedProtocols: SupportedProtocols(),
			Identity:           identity,
		},
		MessageMap: make(map[bitcoin.Hash32]int),
	}
}

func NewInitiationChannelCommunication(peerChannels channels.PeerChannels) *ChannelCommunication {
	return &ChannelCommunication{
		Entity: &channels.Entity{
			PeerChannels: peerChannels,
		},
		MessageMap: make(map[bitcoin.Hash32]int),
	}
}

func NewEmptyChannelCommunication() *ChannelCommunication {
	return &ChannelCommunication{
		Entity:     nil,
		MessageMap: make(map[bitcoin.Hash32]int),
	}
}

func (c *ChannelCommunication) HasPeerChannelID(id string) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, peerChannel := range c.Entity.PeerChannels {
		if peerChannel.ID == id {
			return true
		}
	}

	return false
}

func (c *ChannelCommunication) GetPublicKey() *bitcoin.PublicKey {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.Entity != nil {
		return &c.Entity.PublicKey
	}

	return nil
}

func (c *ChannelCommunication) SetEntity(entity *channels.Entity) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.Entity != nil {
		js, _ := json.MarshalIndent(c.Entity, "", "  ")
		fmt.Printf("Entity : %s\n", js)
		return ErrAlreadyHaveEntity
	}

	c.Entity = entity
	return nil
}

func (c *ChannelCommunication) GetUnprocessedMessages(ctx context.Context) (Messages, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	var result Messages
	for _, message := range c.Messages {
		if message.IsProcessed {
			continue
		}

		msg := *message
		result = append(result, &msg)
	}

	return result, nil
}

func (c *ChannelCommunication) AddMessage(ctx context.Context, message bitcoin.Script) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	now := channels.Now()
	msg := &Message{
		Payload:  message,
		Received: &now,
	}

	hash := bitcoin.Hash32(sha256.Sum256(message))

	c.Messages = append(c.Messages, msg)
	c.MessageMap[hash] = len(c.Messages)

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("message_hash", hash),
	}, "Added message")
	return nil
}

func SendMessage(ctx context.Context, factory *peer_channels.Factory,
	peerChannels channels.PeerChannels, message bitcoin.Script) error {

	success := false
	var lastErr error
	for _, peerChannel := range peerChannels {
		peerClient, err := factory.NewClient(peerChannel.BaseURL)
		if err != nil {
			return errors.Wrap(err, "peer client")
		}

		if _, err := peerClient.PostBinaryMessage(ctx, peerChannel.ID, peerChannel.WriteToken,
			message); err != nil {
			logger.WarnWithFields(ctx, []logger.Field{
				logger.String("base_url", peerChannel.BaseURL),
				logger.String("channel", peerChannel.ID),
			}, "Failed to post peer channel message : %s", err)
			lastErr = err
		} else {
			success = true
		}
	}

	if !success {
		return lastErr
	}

	return nil
}

func (c *Channel) IsInitiation() bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.isInitiation
}

func (c *Channel) Hash() bitcoin.Hash32 {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.hash
}

func (c *Channel) Reject(ctx context.Context, message *peer_channels.Message,
	reject *channels.Reject) error {

	reject.MessageHash = message.Hash()

	logger.InfoWithFields(ctx, []logger.Field{
		logger.String("channel", message.ChannelID),
		logger.String("content_type", message.ContentType),
		logger.Uint32("sequence", message.Sequence),
		logger.Stringer("received", message.Received),
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
	return c.Outgoing.AddMessage(ctx, script)
}

func (c *Channel) ProcessMessage(ctx context.Context, message *peer_channels.Message) error {
	if err := c.Incoming.AddMessage(ctx, bitcoin.Script(message.Payload)); err != nil {
		return errors.Wrap(err, "add message")
	}

	wMessage, err := channels.Unwrap(bitcoin.Script(message.Payload))
	if err != nil {
		return errors.Wrap(err, "unwrap")
	}

	if wMessage.Signature == nil {
		if err := c.Reject(ctx, message, &channels.Reject{
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

	publicKey := c.Outgoing.GetPublicKey()

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
		if err := c.Reject(ctx, message, &channels.Reject{
			Reason:           channels.RejectReasonInvalid,
			RejectProtocolID: channels.ProtocolIDRelationships,
			Code:             channels.RelationshipsRejectCodeNotInitiated,
		}); err != nil {
			return errors.Wrap(err, "no signature: reject")
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
		if err := c.Reject(ctx, message, &channels.Reject{
			Reason:           channels.RejectReasonInvalid,
			RejectProtocolID: channels.ProtocolIDSignedMessages,
			Code:             code,
		}); err != nil {
			return errors.Wrap(err, "reject")
		}
		return nil
	}

	if !c.IsInitiation() && entity != nil {
		if err := c.Outgoing.SetEntity(entity); err != nil {
			// TODO Allow entity updates --ce
			if errors.Cause(err) == ErrAlreadyHaveEntity {
				fmt.Printf("Already have entity\n")
				if err := c.Reject(ctx, message, &channels.Reject{
					Reason:           channels.RejectReasonInvalid,
					RejectProtocolID: channels.ProtocolIDRelationships,
					Code:             channels.RelationshipsRejectCodeAlreadyInitiated,
				}); err != nil {
					return errors.Wrap(err, "reject")
				}
			} else {
				return errors.Wrap(err, "set entity")
			}
		}
	}

	return nil
}
