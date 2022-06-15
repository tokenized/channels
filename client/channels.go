package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/invoices"
	"github.com/tokenized/channels/merkle_proofs"
	"github.com/tokenized/channels/relationships"
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

	// externalID is an ID used by the higher level application
	externalID *bitcoin.Hash32

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
		typ:                 typ,
		hash:                hash,
		key:                 key,
		incoming:            NewCommunicationChannel(incomingPeerChannels, store, channelIncomingPath(hash)),
		outgoing:            NewCommunicationChannel(nil, store, channelOutgoingPath(hash)),
		store:               store,
		peerChannelsFactory: peerChannelsFactory,
	}
}

func newChannel(hash bitcoin.Hash32, key bitcoin.Key, store storage.StreamReadWriter,
	peerChannelsFactory *peer_channels.Factory) *Channel {
	return &Channel{
		hash:                hash,
		key:                 key,
		incoming:            newCommunicationChannel(store, channelIncomingPath(hash)),
		outgoing:            newCommunicationChannel(store, channelOutgoingPath(hash)),
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

func (c *Channel) SetExternalID(id bitcoin.Hash32) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	c.externalID = &id
}

func (c *Channel) ExternalID() *bitcoin.Hash32 {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.externalID
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

func (c *Channel) GetIncomingPeerChannel(id string) *channels.PeerChannel {
	peerChannels := c.incoming.PeerChannels()
	for _, peerChannel := range peerChannels {
		if peerChannel.ID == id {
			return &peerChannel
		}
	}

	return nil
}

func (c *Channel) InitializeRelationship(ctx context.Context, protocols *channels.Protocols,
	payload bitcoin.Script, publicKey bitcoin.PublicKey, outgoing channels.PeerChannels) error {

	wMessage, err := protocols.Unwrap(payload)
	if err != nil {
		return errors.Wrap(err, "unwrap")
	}

	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))

	if c.Type() != ChannelTypeRelationship {
		return ErrNotRelationship
	}

	msg, err := c.incoming.AddMessage(ctx, payload, wMessage)
	if err != nil {
		return errors.Wrap(err, "add message")
	}

	if err := c.incoming.MarkMessageIsProcessed(ctx, msg.ID()); err != nil {
		return errors.Wrap(err, "mark processed")
	}

	if err := c.outgoing.SetPeerChannels(outgoing); err != nil {
		return errors.Wrap(err, "set entity")
	}

	if err := c.SetExternalPublicKey(ctx, publicKey); err != nil {
		return errors.Wrap(err, "set outgoing public key")
	}

	if err := c.MarkMessageIsProcessed(ctx, msg.ID()); err != nil {
		return errors.Wrap(err, "mark processed")
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("channel_hash", c.Hash()),
		logger.Stringer("public_key", publicKey),
	}, "Relationship initiated directly")

	return nil
}

// GetIncomingMessage returns the message for the specified id. It returns a copy so the message
// modification functions will not work.
func (c *Channel) GetIncomingMessage(ctx context.Context, id uint64) (*Message, error) {
	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))
	return c.incoming.GetMessage(ctx, id)
}

// GetOutgoingMessage returns the message for the specified id. It returns a copy so the message
// modification functions will not work.
func (c *Channel) GetOutgoingMessage(ctx context.Context, id uint64) (*Message, error) {
	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))
	return c.outgoing.GetMessage(ctx, id)
}

func (c *Channel) CreateMessage(ctx context.Context, msg channels.Writer,
	responseID *uint64) (*Message, error) {

	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))

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
	responseID *uint64) (uint64, error) {

	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))

	message, err := c.CreateMessage(ctx, msg, responseID)
	if err != nil {
		return 0, errors.Wrap(err, "create")
	}

	if err := c.outgoing.sendMessage(ctx, c.peerChannelsFactory, message); err != nil {
		return 0, errors.Wrap(err, "send")
	}

	if responseID != nil {
		if err := c.MarkMessageIsProcessed(ctx, *responseID); err != nil {
			return 0, errors.Wrap(err, "mark processed")
		}
	}

	return message.ID(), nil
}

func (c *Channel) NewMessage(ctx context.Context) (*Message, error) {
	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))
	return c.outgoing.newMessage(ctx)
}

func (c *Channel) SetMessageIsAwaitingResponse(ctx context.Context, id uint64) error {
	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))
	return c.outgoing.SetMessageIsAwaitingResponse(ctx, id)
}

func (c *Channel) ClearMessageIsAwaitingResponse(ctx context.Context, id uint64) error {
	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))
	return c.outgoing.ClearMessageIsAwaitingResponse(ctx, id)
}

func (c *Channel) MarkMessageIsProcessed(ctx context.Context, id uint64) error {
	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))
	return c.incoming.MarkMessageIsProcessed(ctx, id)
}

func (c *Channel) GetExternalPublicKey() *bitcoin.PublicKey {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.externalPublicKey
}

func (c *Channel) SetExternalPublicKey(ctx context.Context, publicKey bitcoin.PublicKey) error {
	c.lock.RLock()
	defer c.lock.RUnlock()

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("channel_hash", c.Hash()),
		logger.Stringer("public_key", publicKey),
	}, "Setting channel external public key")

	c.externalPublicKey = &publicKey
	return nil
}

func (c *Channel) ProcessMessage(ctx context.Context, protocols *channels.Protocols,
	wallet Wallet, message *peer_channels.Message) (*Message, error) {

	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))

	wrap, err := protocols.Unwrap(bitcoin.Script(message.Payload))
	if err != nil {
		return nil, errors.Wrap(err, "unwrap")
	}

	msg, err := c.incoming.AddMessage(ctx, bitcoin.Script(message.Payload), wrap)
	if err != nil {
		return nil, errors.Wrap(err, "add message")
	}

	if wrap.Signature == nil {
		if err := msg.Reject(&channels.Response{
			Status:         channels.StatusReject,
			CodeProtocolID: channels.ProtocolIDSignedMessages,
			Code:           channels.SignedStatusSignatureRequired,
		}); err != nil {
			return nil, errors.Wrap(err, "no signature: reject")
		}
		return nil, nil
	}

	if wrap.Response != nil {
		logger.InfoWithFields(ctx, []logger.Field{
			logger.Uint64("response_id", wrap.Response.MessageID),
		}, "Response")
	}

	if initiation, ok := wrap.Message.(*relationships.Initiation); ok {
		if err := c.relationshipInitiation(ctx, wallet, msg, wrap, initiation); err != nil {
			return nil, errors.Wrap(err, "relationship initiation")
		}

		return msg, nil
	}

	publicKey := c.GetExternalPublicKey()
	if publicKey == nil {
		if err := msg.Reject(&channels.Response{
			Status:         channels.StatusReject,
			CodeProtocolID: relationships.ProtocolID,
			Code:           relationships.StatusNotInitiated,
		}); err != nil {
			return nil, errors.Wrap(err, "no relationship: reject")
		}

		return nil, nil
	}

	if wrap.Signature.PublicKey != nil {
		if !wrap.Signature.PublicKey.Equal(*publicKey) {
			return nil, ErrWrongPublicKey
		}
	} else {
		wrap.Signature.SetPublicKey(publicKey)
	}

	if err := wrap.Signature.Verify(); err != nil {
		var code uint32
		if errors.Cause(err) == channels.ErrInvalidSignature {
			code = channels.SignedStatusInvalidSignature
		}
		if err := msg.Reject(&channels.Response{
			Status:         channels.StatusInvalid,
			CodeProtocolID: channels.ProtocolIDSignedMessages,
			Code:           code,
		}); err != nil {
			return nil, errors.Wrap(err, "reject")
		}

		return nil, nil
	}

	switch channelsMsg := wrap.Message.(type) {
	case *merkle_proofs.MerkleProof:
		if err := c.merkleProof(ctx, wallet, msg, wrap, channelsMsg); err != nil {
			return nil, errors.Wrap(err, "merkle proof")
		}

	case *invoices.TransferAccept:
		if err := c.transferAccept(ctx, protocols, wallet, msg, wrap,
			channelsMsg); err != nil {
			return nil, errors.Wrap(err, "transfer accept")
		}

		// Note: Other messages need to be processed outside this client.
	}

	return msg, nil
}

func (c *Channel) relationshipInitiation(ctx context.Context, wallet Wallet, msg *Message,
	wrap *channels.WrappedMessage, initiation *relationships.Initiation) error {

	publicKey := c.GetExternalPublicKey()
	if publicKey != nil {
		if err := msg.Reject(&channels.Response{
			Status:         channels.StatusReject,
			CodeProtocolID: relationships.ProtocolID,
			Code:           relationships.StatusAlreadyInitiated,
		}); err != nil {
			return errors.Wrap(err, "relationship already initiated: reject")
		}

		return nil
	}

	if wrap.Signature.PublicKey != nil {
		if !wrap.Signature.PublicKey.Equal(initiation.Configuration.PublicKey) {
			return ErrWrongPublicKey
		}
	} else {
		wrap.Signature.SetPublicKey(&initiation.Configuration.PublicKey)
	}

	if err := wrap.Signature.Verify(); err != nil {
		var code uint32
		if errors.Cause(err) == channels.ErrInvalidSignature {
			code = channels.SignedStatusInvalidSignature
		}

		if err := msg.Reject(&channels.Response{
			Status:         channels.StatusReject,
			CodeProtocolID: channels.ProtocolIDSignedMessages,
			Code:           code,
		}); err != nil {
			return errors.Wrap(err, "signature: reject")
		}

		return nil
	}

	if c.Type() != ChannelTypeRelationship {
		// This initiation must be manually accepted and setup on another channel.
		logger.Info(ctx, "Relationship initiation received")
		return nil
	}

	if err := c.outgoing.SetPeerChannels(initiation.Configuration.PeerChannels); err != nil {
		return errors.Wrap(err, "set outgoing peer channels")
	}

	if err := c.SetExternalPublicKey(ctx, initiation.Configuration.PublicKey); err != nil {
		return errors.Wrap(err, "set external public key")
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("channel_hash", c.Hash()),
		logger.Stringer("public_key", initiation.Configuration.PublicKey),
	}, "Relationship initiated via response")

	if err := c.incoming.MarkMessageIsProcessed(ctx, msg.ID()); err != nil {
		return errors.Wrap(err, "mark is processed")
	}

	return nil
}

func (c *Channel) merkleProof(ctx context.Context, wallet Wallet, msg *Message,
	wrap *channels.WrappedMessage, merkleProof *merkle_proofs.MerkleProof) error {
	if wallet == nil {
		return nil
	}

	if _, err := wallet.AddMerkleProof(ctx, merkleProof.MerkleProof); err != nil {
		return errors.Wrap(err, "add merkle proof")
	}

	if err := c.incoming.MarkMessageIsProcessed(ctx, msg.ID()); err != nil {
		return errors.Wrap(err, "mark is processed")
	}

	return nil
}

func (c *Channel) transferAccept(ctx context.Context, protocols *channels.Protocols,
	wallet Wallet, msg *Message, wrap *channels.WrappedMessage,
	transferAccept *invoices.TransferAccept) error {

	if wallet == nil {
		return nil
	}

	if transferAccept.Tx != nil {
		if err := wallet.AddTx(ctx, c.Hash(), transferAccept.Tx.Tx); err != nil {
			return errors.Wrap(err, "add tx")
		}

		if err := c.incoming.MarkMessageIsProcessed(ctx, msg.ID()); err != nil {
			return errors.Wrap(err, "mark is processed")
		}

		return nil
	}

	// Find tx in Transfer to which the TransferAccept is a response.
	if wrap.Response == nil {
		if err := msg.Reject(&channels.Response{
			Status:         channels.StatusInvalid,
			CodeProtocolID: invoices.ProtocolID,
			Code:           invoices.StatusMissingResponseID,
			Note:           "TransferAccept missing response id",
		}); err != nil {
			return errors.Wrap(err, "response id missing: reject")
		}
		return nil
	}

	transferMsg, err := c.outgoing.GetMessage(ctx, wrap.Response.MessageID)
	if err != nil {
		if errors.Cause(err) == ErrMessageNotFound {
			if err := msg.Reject(&channels.Response{
				Status:         channels.StatusInvalid,
				CodeProtocolID: channels.ProtocolIDResponse,
				Code:           channels.ResponseStatusMessageNotFound,
				Note:           "Response id message not found",
			}); err != nil {
				return errors.Wrap(err, "response id not found: reject")
			}
			return nil
		}

		return errors.Wrap(err, "get message")
	}

	transferWrap, err := protocols.Unwrap(bitcoin.Script(transferMsg.Payload()))
	if err != nil {
		return errors.Wrap(err, "unwrap")
	}

	transfer, ok := transferWrap.Message.(*invoices.Transfer)
	if !ok {
		if err := msg.Reject(&channels.Response{
			Status:         channels.StatusInvalid,
			CodeProtocolID: channels.ProtocolIDResponse,
			Code:           channels.ResponseStatusWrongMessage,
			Note:           "Response message not a transfer",
		}); err != nil {
			return errors.Wrap(err, "response id not transfer: reject")
		}
		return nil
	}

	if err := wallet.AddTx(ctx, c.Hash(), transfer.Tx.Tx); err != nil {
		return errors.Wrap(err, "add tx")
	}

	if err := c.incoming.MarkMessageIsProcessed(ctx, msg.ID()); err != nil {
		return errors.Wrap(err, "mark is processed")
	}

	return nil
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
	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", c.Hash()))

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

	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("channel_hash", hash))

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

	if err := binary.Write(w, endian, c.externalID != nil); err != nil {
		return errors.Wrap(err, "has external id")
	}

	if c.externalID != nil {
		if err := c.externalID.Serialize(w); err != nil {
			return errors.Wrap(err, "external id")
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

	var hasExternalID bool
	if err := binary.Read(r, endian, &hasExternalID); err != nil {
		return errors.Wrap(err, "has external id")
	}

	if hasExternalID {
		c.externalID = &bitcoin.Hash32{}
		if err := c.externalID.Deserialize(r); err != nil {
			return errors.Wrap(err, "external id")
		}
	} else {
		c.externalID = nil
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
