package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/wallet"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/threads"

	"github.com/pkg/errors"
)

const (
	clientsPath    = "channels_client"
	clientsVersion = uint8(0)
)

var (
	ErrSignatureRequired   = errors.New("Signature Required")
	ErrMissingRelationship = errors.New("Missing Relationship")
	ErrWrongPublicKey      = errors.New("Wrong Public Key")
	ErrMessageNotFound     = errors.New("Message Not Found")

	endian = binary.LittleEndian
)

type Client struct {
	baseKey           bitcoin.Key
	peerAccountClient peer_channels.AccountClient
	channels          Channels

	store               storage.StreamReadWriter
	peerChannelsFactory *peer_channels.Factory

	messagesLock    sync.Mutex
	messagesChannel chan ChannelMessage

	channelHashes []bitcoin.Hash32 // used to load channels from storage

	lock sync.RWMutex
}

func SupportedProtocols() envelope.ProtocolIDs {
	return envelope.ProtocolIDs{
		channels.ProtocolIDResponse,
		channels.ProtocolIDReject,
		channels.ProtocolIDSignedMessages,
		channels.ProtocolIDRelationships,
		channels.ProtocolIDInvoices,
		channels.ProtocolIDMerkleProof,
		channels.ProtocolIDPeerChannels,
	}
}

func NewClient(baseKey bitcoin.Key, peerAccountClient peer_channels.AccountClient,
	store storage.StreamReadWriter, peerChannelsFactory *peer_channels.Factory) *Client {
	return &Client{
		baseKey:             baseKey,
		peerAccountClient:   peerAccountClient,
		store:               store,
		peerChannelsFactory: peerChannelsFactory,
	}
}

func (c *Client) BaseKey() bitcoin.Key {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.baseKey
}

func (c *Client) Channel(index int) *Channel {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.channels[index]
}

func (c *Client) GetChannel(channelID string) (*Channel, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, channel := range c.channels {
		if channel.incoming.HasPeerChannelID(channelID) {
			return channel, nil
		}
	}

	return nil, nil
}

// CreatePublicChannel creates a new channel to share publicly so other users can initiation
// relationships. Those relationships will be
func (c *Client) CreateRelationshipInitiationChannel(ctx context.Context,
	contextID bitcoin.Hash32) (*Channel, error) {

	peerChannel, err := c.peerAccountClient.CreatePublicChannel(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "create peer channel")
	}

	peerChannels := channels.PeerChannels{
		{
			BaseURL: c.peerAccountClient.BaseURL(),
			ID:      peerChannel.ID,
		},
	}

	hash, key := wallet.GenerateHashKey(c.BaseKey(), contextID)
	channel := NewChannel(ChannelTypeRelationshipInitiation, hash, key, peerChannels, c.store,
		c.peerChannelsFactory)

	c.lock.Lock()
	c.channels = append(c.channels, channel)
	c.lock.Unlock()

	return channel, nil
}

// RegisterRelationshipInitiationChannel registers an existing public channel with the client.
func (c *Client) RegisterRelationshipInitiationChannel(ctx context.Context,
	contextID bitcoin.Hash32, peerChannels channels.PeerChannels) (*Channel, error) {

	hash, key := wallet.GenerateHashKey(c.BaseKey(), contextID)
	channel := NewChannel(ChannelTypeRelationshipInitiation, hash, key, peerChannels, c.store,
		c.peerChannelsFactory)

	c.lock.Lock()
	c.channels = append(c.channels, channel)
	c.lock.Unlock()

	return channel, nil
}

func (c *Client) CreateRelationshipChannel(ctx context.Context,
	contextID bitcoin.Hash32) (*Channel, error) {

	peerChannel, err := c.peerAccountClient.CreateChannel(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "create peer channel")
	}

	peerChannels := channels.PeerChannels{
		{
			BaseURL:    c.peerAccountClient.BaseURL(),
			ID:         peerChannel.ID,
			WriteToken: peerChannel.GetWriteToken(),
		},
	}

	hash, key := wallet.GenerateHashKey(c.BaseKey(), contextID)
	channel := NewChannel(ChannelTypeRelationship, hash, key, peerChannels, c.store,
		c.peerChannelsFactory)

	c.lock.Lock()
	c.channels = append(c.channels, channel)
	c.lock.Unlock()

	return channel, nil
}

// RegisterRelationshipChannel registers an existing channel with the client.
func (c *Client) RegisterRelationshipChannel(ctx context.Context,
	contextID bitcoin.Hash32, peerChannels channels.PeerChannels) (*Channel, error) {

	hash, key := wallet.GenerateHashKey(c.BaseKey(), contextID)
	channel := NewChannel(ChannelTypeRelationship, hash, key, peerChannels, c.store,
		c.peerChannelsFactory)

	c.lock.Lock()
	c.channels = append(c.channels, channel)
	c.lock.Unlock()

	return channel, nil
}

func (c *Client) GetUnprocessedMessages(ctx context.Context) (ChannelMessages, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	var result ChannelMessages
	for i, channel := range c.channels {
		messages, err := channel.incoming.GetUnprocessedMessages(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "channel %d", i)
		}

		for _, message := range messages {
			result = append(result, &ChannelMessage{
				Message: *message,
				Channel: channel,
			})
		}
	}

	return result, nil
}

// GetIncomingChannel returns a channel that is fed all new incoming messages. CloseIncomingChannel
// must be called to close it.
func (c *Client) GetIncomingChannel(ctx context.Context) <-chan ChannelMessage {
	c.messagesLock.Lock()
	defer c.messagesLock.Unlock()

	c.messagesChannel = make(chan ChannelMessage, 100)
	return c.messagesChannel
}

// CloseIncomingChannel closes a channel previously retrieved with GetIncomingChannel.
func (c *Client) CloseIncomingChannel(ctx context.Context) {
	c.messagesLock.Lock()
	defer c.messagesLock.Unlock()

	if c.messagesChannel != nil {
		close(c.messagesChannel)
		c.messagesChannel = nil
	}
}

func (c *Client) Run(ctx context.Context, interrupt <-chan interface{}) error {
	wait := &sync.WaitGroup{}
	incomingMessages := make(chan peer_channels.Message)

	listenThread := threads.NewThread("Listen for Messages", func(ctx context.Context,
		interrupt <-chan interface{}) error {
		return c.peerAccountClient.Listen(ctx, incomingMessages, interrupt)
	})
	listenThread.SetWait(wait)
	listenThreadComplete := listenThread.GetCompleteChannel()

	handleThread := threads.NewThreadWithoutStop("Handle Messages",
		func(ctx context.Context) error {
			return c.handleMessages(ctx, incomingMessages)
		})
	handleThread.SetWait(wait)
	handleThreadComplete := handleThread.GetCompleteChannel()

	listenThread.Start(ctx)
	handleThread.Start(ctx)

	select {
	case <-interrupt:
		listenThread.Stop(ctx)
		close(incomingMessages)

	case <-listenThreadComplete:
		logger.Warn(ctx, "Listen for messages thread stopped : %s", listenThread.Error())
		listenThread.Stop(ctx)
		close(incomingMessages)

	case <-handleThreadComplete:
		logger.Warn(ctx, "Handle messages thread stopped : %s", handleThread.Error())
		listenThread.Stop(ctx)
	}

	wait.Wait()
	return listenThread.Error()
}

func (c *Client) handleMessages(ctx context.Context, incoming <-chan peer_channels.Message) error {
	for message := range incoming {
		if err := c.handleMessage(ctx, &message); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) handleMessage(ctx context.Context, message *peer_channels.Message) error {
	logger.VerboseWithFields(ctx, []logger.Field{
		logger.String("channel", message.ChannelID),
		logger.String("content_type", message.ContentType),
		logger.Uint32("sequence", message.Sequence),
		logger.Stringer("received", message.Received),
		logger.Stringer("message_hash", message.Hash()),
	}, "Received message")

	if message.ContentType != peer_channels.ContentTypeBinary {
		logger.WarnWithFields(ctx, []logger.Field{
			logger.String("channel", message.ChannelID),
			logger.String("content_type", message.ContentType),
			logger.Uint32("sequence", message.Sequence),
			logger.Stringer("received", message.Received),
			logger.Stringer("message_hash", message.Hash()),
		}, "Message content not binary")
		return nil
	}

	channel, err := c.GetChannel(message.ChannelID)
	if err != nil {
		return errors.Wrap(err, "get channel")
	}
	if channel == nil {
		logger.WarnWithFields(ctx, []logger.Field{
			logger.String("channel", message.ChannelID),
			logger.Uint32("sequence", message.Sequence),
			logger.Stringer("received", message.Received),
			logger.Stringer("message_hash", message.Hash()),
		}, "Unknown channel")
		return nil
	}

	if err := c.processMessage(ctx, channel, message); err != nil {
		logger.WarnWithFields(ctx, []logger.Field{
			logger.String("channel", message.ChannelID),
			logger.Uint32("sequence", message.Sequence),
			logger.Stringer("received", message.Received),
			logger.Stringer("message_hash", message.Hash()),
		}, "Process message : %s", err)
		return nil
	}

	return nil
}

func (c *Client) processMessage(ctx context.Context, channel *Channel,
	message *peer_channels.Message) error {

	result, err := channel.ProcessMessage(ctx, message)
	if err != nil {
		return errors.Wrap(err, "channel")
	}

	if result != nil {
		c.messagesLock.Lock()
		if c.messagesChannel != nil {
			c.messagesChannel <- ChannelMessage{
				Message: *result,
				Channel: channel,
			}
		}
		c.messagesLock.Unlock()
	}

	return nil
}

func (c *Client) Save(ctx context.Context) error {
	path := fmt.Sprintf("%s/%s", clientsPath, c.BaseKey().PublicKey())

	if err := storage.StreamWrite(ctx, c.store, path, c); err != nil {
		return errors.Wrap(err, "write")
	}

	c.lock.RLock()
	defer c.lock.RUnlock()

	for i, channel := range c.channels {
		if err := channel.Save(ctx); err != nil {
			return errors.Wrapf(err, "channel %d: %s", i, channel.Hash())
		}
	}

	return nil
}

func (c *Client) Load(ctx context.Context) error {
	path := fmt.Sprintf("%s/%s", clientsPath, c.BaseKey().PublicKey())

	if err := storage.StreamRead(ctx, c.store, path, c); err != nil {
		if errors.Cause(err) == storage.ErrNotFound {
			return nil
		}
		return errors.Wrap(err, "read")
	}

	// Load channels
	c.channels = make(Channels, len(c.channelHashes))
	for i, channelHash := range c.channelHashes {
		channelKey, err := c.BaseKey().AddHash(channelHash)
		if err != nil {
			return errors.Wrap(err, "add hash")
		}

		channel, err := LoadChannel(ctx, c.store, c.peerChannelsFactory, channelHash, channelKey)
		if err != nil {
			return errors.Wrapf(err, "channel %d: %s", i, channelHash)
		}

		c.channels[i] = channel
	}

	return nil
}

func (c *Client) Serialize(w io.Writer) error {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if err := binary.Write(w, endian, clientsVersion); err != nil {
		return errors.Wrap(err, "version")
	}

	if err := binary.Write(w, endian, uint32(len(c.channels))); err != nil {
		return errors.Wrap(err, "channel count")
	}

	for i, channel := range c.channels {
		hash := channel.Hash()
		if _, err := w.Write(hash[:]); err != nil {
			return errors.Wrapf(err, "channel %d", i)
		}
	}

	return nil
}

func (c *Client) Deserialize(r io.Reader) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	var version uint8
	if err := binary.Read(r, endian, &version); err != nil {
		return errors.Wrap(err, "version")
	}
	if version != 0 {
		return errors.New("Unsupported version")
	}

	var channelCount uint32
	if err := binary.Read(r, endian, &channelCount); err != nil {
		return errors.Wrap(err, "channel count")
	}

	c.channelHashes = make([]bitcoin.Hash32, channelCount)
	for i := range c.channelHashes {
		if _, err := io.ReadFull(r, c.channelHashes[i][:]); err != nil {
			return errors.Wrapf(err, "channel %d", i)
		}
	}

	return nil
}
