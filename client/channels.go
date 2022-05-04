package client

import (
	"context"
	"crypto/sha256"
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

	Incoming *ChannelCommunication
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
		channels.ProtocolIDChannels,
		channels.ProtocolIDRelationships,
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

func (c *ChannelCommunication) AddMessage(ctx context.Context, hash bitcoin.Hash32,
	protocolIDs envelope.ProtocolIDs, payload bitcoin.ScriptItems) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	now := channels.Now()
	message := &Message{
		ProtocolIDs: protocolIDs,
		Payload:     payload,
		Received:    &now,
	}

	c.Messages = append(c.Messages, message)
	c.MessageMap[hash] = len(c.Messages)

	logger.Info(ctx, "Added message")
	return nil
}

func SendMessage(ctx context.Context, factory *peer_channels.Factory,
	peerChannels channels.PeerChannels, protocolIDs envelope.ProtocolIDs,
	payload bitcoin.ScriptItems) (*bitcoin.Hash32, error) {

	scriptItems := envelopeV1.Wrap(protocolIDs, payload)
	script, err := scriptItems.Script()
	if err != nil {
		return nil, errors.Wrap(err, "script")
	}

	hash := bitcoin.Hash32(sha256.Sum256(script))

	success := false
	var lastErr error
	for _, peerChannel := range peerChannels {
		peerClient, err := factory.NewClient(peerChannel.BaseURL)
		if err != nil {
			return nil, errors.Wrap(err, "peer client")
		}

		if _, err := peerClient.PostBinaryMessage(ctx, peerChannel.ID, peerChannel.WriteToken,
			script); err != nil {
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
		return nil, lastErr
	}

	return &hash, nil
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

func (c *Channel) Reject(ctx context.Context, message peer_channels.Message,
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

	protocolIDs, payload, err := channels.WriteChannels(reject, nil, nil)
	if err != nil {
		return errors.Wrap(err, "write")
	}

	scriptItems := envelopeV1.Wrap(protocolIDs, payload)
	hasher := sha256.New()
	scriptItems.Write(hasher)
	hash, _ := bitcoin.NewHash32(hasher.Sum(nil))

	return c.Outgoing.AddMessage(ctx, *hash, protocolIDs, payload)
}

func (c *Channel) ProcessMessage(ctx context.Context, message peer_channels.Message,
	protocolIDs envelope.ProtocolIDs, payload bitcoin.ScriptItems) error {

	fullProtocolIDs := protocolIDs
	fullPayload := payload

	// Check signatures
	var signature *channels.Signature
	var err error
	signature, protocolIDs, payload, err = channels.ParseSigned(protocolIDs, payload)
	if err != nil {
		if err := c.Reject(ctx, message, &channels.Reject{
			Reason:     channels.RejectReasonInvalid,
			ProtocolID: channels.ProtocolIDSignedMessages,
		}); err != nil {
			return errors.Wrap(err, "parse signature")
		}
		return nil
	}
	if signature == nil {
		if err := c.Reject(ctx, message, &channels.Reject{
			Reason:     channels.RejectReasonInvalid,
			ProtocolID: channels.ProtocolIDSignedMessages,
			Code:       channels.SignedRejectCodeSignatureRequired,
		}); err != nil {
			return errors.Wrap(err, "no signature: reject")
		}
		return nil
	}

	if len(protocolIDs) == 0 {
		return errors.New("Not Enough Protocol IDs")
	}

	publicKey := c.Outgoing.GetPublicKey()

	var entity *channels.Entity
	msg, err := channels.ParseRelationship(protocolIDs, payload)
	if err != nil {
		return errors.Wrap(err, "parse")
	}
	if msg != nil {
		switch message := msg.(type) {
		case *channels.RelationshipInitiation:
			relationship := channels.Entity(*message)
			entity = &relationship
		default:
			return channels.ErrUnsupportedRelationshipsMessage
		}

		if publicKey == nil && entity != nil {
			// Use newly established relationship key
			publicKey = &entity.PublicKey
		}
	}

	if publicKey == nil {
		if err := c.Reject(ctx, message, &channels.Reject{
			Reason:     channels.RejectReasonInvalid,
			ProtocolID: channels.ProtocolIDRelationships,
			Code:       channels.RelationshipsRejectCodeNotInitiated,
		}); err != nil {
			return errors.Wrap(err, "no signature: reject")
		}
		return nil
	}

	if signature.PublicKey != nil {
		if !signature.PublicKey.Equal(*publicKey) {
			return ErrWrongPublicKey
		}
	} else {
		signature.SetPublicKey(publicKey)
	}

	if err := signature.Verify(); err != nil {
		var code uint32
		if errors.Cause(err) == channels.ErrInvalidSignature {
			code = channels.SignedRejectCodeInvalidSignature
		}
		if err := c.Reject(ctx, message, &channels.Reject{
			Reason:     channels.RejectReasonInvalid,
			ProtocolID: channels.ProtocolIDSignedMessages,
			Code:       code,
		}); err != nil {
			return errors.Wrap(err, "reject")
		}
		return nil
	}

	if !c.IsInitiation() && entity != nil {
		if err := c.Incoming.SetEntity(entity); err != nil {
			// TODO Allow entity updates --ce
			if errors.Cause(err) == ErrAlreadyHaveEntity {
				if err := c.Reject(ctx, message, &channels.Reject{
					Reason:     channels.RejectReasonInvalid,
					ProtocolID: channels.ProtocolIDRelationships,
					Code:       channels.RelationshipsRejectCodeAlreadyInitiated,
				}); err != nil {
					return errors.Wrap(err, "reject")
				}
			} else {
				return errors.Wrap(err, "set entity")
			}
		}
	}

	return c.Incoming.AddMessage(ctx, message.Hash(), fullProtocolIDs, fullPayload)
}
