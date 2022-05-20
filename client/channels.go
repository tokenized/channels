package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/wallet"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"

	"github.com/pkg/errors"
)

const (
	ChannelTypeUnspecified = ChannelType(0)

	// ChannelTypeRelationshipInitiation is the channel type that is used to initiation new
	// relationships. Relationship initiation messages should be received on it, then a new
	// ChannelTypeRelationship channel is created for each relationship.
	ChannelTypeRelationshipInitiation = ChannelType(1)

	// ChannelTypeRelationship is a channel that is used for one relationship.
	ChannelTypeRelationship = ChannelType(2)

	channelsPath    = "channels_client/channels"
	channelsVersion = uint8(0)
)

var (
	ErrAlreadyEstablished = errors.New("Already Established")
	ErrNotRelationship    = errors.New("Not Relationship")
)

type ChannelType uint8

type Channel struct {
	// typ is the type of the channel, or what it is used for.
	typ ChannelType

	// Hash used to derive channel's base key
	hash bitcoin.Hash32

	// Base key used to sign messages.
	key bitcoin.Key

	// externalPublicKey is the base public key of the other party.
	externalPublicKey *bitcoin.PublicKey

	// incoming represents the messages are received.
	incoming *CommunicationChannel

	// outgoing represents the messages are sent.
	outgoing *CommunicationChannel

	store               storage.StreamReadWriter
	peerChannelsFactory *peer_channels.Factory

	lock sync.RWMutex
}

type Channels []*Channel

func NewChannel(typ ChannelType, hash bitcoin.Hash32, key bitcoin.Key,
	incomingPeerChannels channels.PeerChannels, store storage.StreamReadWriter,
	peerChannelsFactory *peer_channels.Factory) *Channel {

	return &Channel{
		typ:  typ,
		hash: hash,
		key:  key,
		incoming: NewCommunicationChannel(incomingPeerChannels, store,
			channelIncomingPath(hash)),
		outgoing: NewCommunicationChannel(nil, store,
			channelOutgoingPath(hash)),
		store:               store,
		peerChannelsFactory: peerChannelsFactory,
	}
}

func newChannel(hash bitcoin.Hash32, key bitcoin.Key, store storage.StreamReadWriter,
	peerChannelsFactory *peer_channels.Factory) *Channel {
	return &Channel{
		hash: hash,
		key:  key,
		incoming: newCommunicationChannel(store,
			channelIncomingPath(hash)),
		outgoing: newCommunicationChannel(store,
			channelOutgoingPath(hash)),
		store:               store,
		peerChannelsFactory: peerChannelsFactory,
	}
}

func (c *Channel) Type() ChannelType {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.typ
}

func (c *Channel) Hash() bitcoin.Hash32 {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.hash
}

func (c *Channel) Key() bitcoin.Key {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.key
}

func (c *Channel) IncomingPeerChannels() channels.PeerChannels {
	return c.incoming.PeerChannels()
}

func (c *Channel) SetOutgoingPeerChannels(peerChannels channels.PeerChannels) error {
	return c.outgoing.SetPeerChannels(peerChannels)
}

func (c *Channel) InitializeRelationship(ctx context.Context, payload bitcoin.Script,
	initiation *channels.RelationshipInitiation) error {

	if c.Type() != ChannelTypeRelationship {
		return ErrNotRelationship
	}

	if err := c.outgoing.SetPeerChannels(initiation.PeerChannels); err != nil {
		return errors.Wrap(err, "set entity")
	}

	if err := c.SetExternalPublicKey(initiation.PublicKey); err != nil {
		return errors.Wrap(err, "set outgoing public key")
	}

	msg, err := c.incoming.newMessageWithPayload(ctx, payload)
	if err != nil {
		return errors.Wrap(err, "add message")
	}

	if err := c.MarkMessageProcessed(ctx, msg.ID()); err != nil {
		return errors.Wrap(err, "mark processed")
	}

	return nil
}

func (c *Channel) CreateMessage(ctx context.Context, msg channels.Writer,
	responseID *uint64) (*Message, error) {

	newMessage, err := c.NewMessage(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "new message")
	}

	script, err := channels.Wrap(msg, c.Key(), wallet.RandomHash(), newMessage.ID(), responseID)
	if err != nil {
		return nil, errors.Wrap(err, "wrap")
	}
	newMessage.SetPayload(script)

	return newMessage, nil
}

func (c *Channel) SendMessage(ctx context.Context, msg channels.Writer,
	responseID *uint64) error {

	message, err := c.CreateMessage(ctx, msg, responseID)
	if err != nil {
		return errors.Wrap(err, "create")
	}

	if err := c.outgoing.sendMessage(ctx, c.peerChannelsFactory, message); err != nil {
		return errors.Wrap(err, "send")
	}

	if responseID != nil {
		if err := c.MarkMessageProcessed(ctx, *responseID); err != nil {
			return errors.Wrap(err, "mark processed")
		}
	}

	return nil
}

func (c *Channel) NewMessage(ctx context.Context) (*Message, error) {
	return c.outgoing.newMessage(ctx)
}

func (c *Channel) MarkMessageProcessed(ctx context.Context, id uint64) error {
	return c.incoming.MarkMessageProcessed(ctx, id)
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

func (c *Channel) ProcessMessage(ctx context.Context,
	message *peer_channels.Message) (*Message, error) {

	msg, err := c.incoming.newMessageWithPayload(ctx, bitcoin.Script(message.Payload))
	if err != nil {
		return nil, errors.Wrap(err, "add message")
	}

	wMessage, err := channels.Unwrap(bitcoin.Script(message.Payload))
	if err != nil {
		return nil, errors.Wrap(err, "unwrap")
	}

	if wMessage.Signature == nil {
		if err := msg.Reject(&channels.Reject{
			Reason:           channels.RejectReasonInvalid,
			RejectProtocolID: channels.ProtocolIDSignedMessages,
			Code:             channels.SignedRejectCodeSignatureRequired,
		}); err != nil {
			return nil, errors.Wrap(err, "no signature: reject")
		}
		return nil, nil
	}

	if wMessage.Response != nil {
		logger.InfoWithFields(ctx, []logger.Field{
			logger.Uint64("response_id", wMessage.Response.MessageID),
		}, "Response")
	}

	publicKey := c.GetExternalPublicKey()

	var entity *channels.Entity
	switch msg := wMessage.Message.(type) {
	case *channels.RelationshipInitiation:
		relationship := channels.Entity(*msg)
		entity = &relationship
		if publicKey == nil {
			// Use newly established relationship key
			publicKey = &entity.PublicKey
		}
	}

	if publicKey == nil {
		if err := msg.Reject(&channels.Reject{
			Reason:           channels.RejectReasonInvalid,
			RejectProtocolID: channels.ProtocolIDRelationships,
			Code:             channels.RelationshipsRejectCodeNotInitiated,
		}); err != nil {
			return nil, errors.Wrap(err, "no relationship: reject")
		}
		return nil, nil
	}

	if wMessage.Signature.PublicKey != nil {
		if !wMessage.Signature.PublicKey.Equal(*publicKey) {
			return nil, ErrWrongPublicKey
		}
	} else {
		wMessage.Signature.SetPublicKey(publicKey)
	}

	if err := wMessage.Signature.Verify(); err != nil {
		var code uint32
		if errors.Cause(err) == channels.ErrInvalidSignature {
			code = channels.SignedRejectCodeInvalidSignature
		}
		if err := msg.Reject(&channels.Reject{
			Reason:           channels.RejectReasonInvalid,
			RejectProtocolID: channels.ProtocolIDSignedMessages,
			Code:             code,
		}); err != nil {
			return nil, errors.Wrap(err, "reject")
		}
		return nil, nil
	}

	if c.Type() == ChannelTypeRelationship && entity != nil {
		if err := c.outgoing.SetPeerChannels(entity.PeerChannels); err != nil {
			// TODO Allow entity updates --ce
			if errors.Cause(err) == ErrAlreadyEstablished {
				if err := msg.Reject(&channels.Reject{
					Reason:           channels.RejectReasonInvalid,
					RejectProtocolID: channels.ProtocolIDRelationships,
					Code:             channels.RelationshipsRejectCodeAlreadyInitiated,
				}); err != nil {
					return nil, errors.Wrap(err, "already have entity: reject")
				}
			} else {
				return nil, errors.Wrap(err, "set entity")
			}
		}

		if err := c.SetExternalPublicKey(entity.PublicKey); err != nil {
			return nil, errors.Wrap(err, "set outgoing public key")
		}
	}

	return msg, nil
}

func channelPath(hash bitcoin.Hash32) string {
	return fmt.Sprintf("%s/%s/channel", channelsPath, hash)
}

func channelIncomingPath(hash bitcoin.Hash32) string {
	return fmt.Sprintf("%s/%s/incoming", channelsPath, hash)
}

func channelOutgoingPath(hash bitcoin.Hash32) string {
	return fmt.Sprintf("%s/%s/outgoing", channelsPath, hash)
}

func (c *Channel) Save(ctx context.Context) error {
	path := channelPath(c.Hash())

	if err := storage.StreamWrite(ctx, c.store, path, c); err != nil {
		return errors.Wrap(err, "write")
	}

	if err := c.incoming.Save(ctx); err != nil {
		return errors.Wrap(err, "incoming")
	}

	if err := c.outgoing.Save(ctx); err != nil {
		return errors.Wrap(err, "outgoing")
	}

	return nil
}

func LoadChannel(ctx context.Context, store storage.StreamReadWriter,
	peerChannelsFactory *peer_channels.Factory, hash bitcoin.Hash32,
	key bitcoin.Key) (*Channel, error) {

	path := channelPath(hash)
	channel := newChannel(hash, key, store, peerChannelsFactory)
	if err := storage.StreamRead(ctx, store, path, channel); err != nil {
		return nil, errors.Wrap(err, "read")
	}

	if err := channel.incoming.Load(ctx); err != nil {
		return nil, errors.Wrap(err, "incoming")
	}

	if err := channel.outgoing.Load(ctx); err != nil {
		return nil, errors.Wrap(err, "outgoing")
	}

	return channel, nil
}

func (c *Channel) Serialize(w io.Writer) error {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if err := binary.Write(w, endian, channelsVersion); err != nil {
		return errors.Wrap(err, "version")
	}

	if err := binary.Write(w, endian, c.typ); err != nil {
		return errors.Wrap(err, "type")
	}

	if err := binary.Write(w, endian, c.externalPublicKey != nil); err != nil {
		return errors.Wrap(err, "has public key")
	}

	if c.externalPublicKey != nil {
		if err := c.externalPublicKey.Serialize(w); err != nil {
			return errors.Wrap(err, "public key")
		}
	}

	return nil
}

func (c *Channel) Deserialize(r io.Reader) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	var version uint8
	if err := binary.Read(r, endian, &version); err != nil {
		return errors.Wrap(err, "version")
	}
	if version != 0 {
		return errors.New("Unsupported version")
	}

	if err := binary.Read(r, endian, &c.typ); err != nil {
		return errors.Wrap(err, "type")
	}

	var hasPublicKey bool
	if err := binary.Read(r, endian, &hasPublicKey); err != nil {
		return errors.Wrap(err, "has public key")
	}

	if hasPublicKey {
		c.externalPublicKey = &bitcoin.PublicKey{}
		if err := c.externalPublicKey.Deserialize(r); err != nil {
			return errors.Wrap(err, "public key")
		}
	} else {
		c.externalPublicKey = nil
	}

	return nil
}

func (v *ChannelType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for ChannelType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v ChannelType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v ChannelType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown ChannelType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *ChannelType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *ChannelType) SetString(s string) error {
	switch s {
	case "relationship_initiation":
		*v = ChannelTypeRelationshipInitiation
	case "relationship":
		*v = ChannelTypeRelationship
	default:
		*v = ChannelTypeUnspecified
		return fmt.Errorf("Unknown ChannelType value \"%s\"", s)
	}

	return nil
}

func (v ChannelType) String() string {
	switch v {
	case ChannelTypeRelationshipInitiation:
		return "relationship_initiation"
	case ChannelTypeRelationship:
		return "relationship"
	default:
		return ""
	}
}
