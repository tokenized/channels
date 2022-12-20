package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/invoices"
	"github.com/tokenized/channels/merkle_proofs"
	channelsPeerChannels "github.com/tokenized/channels/peer_channels"
	"github.com/tokenized/channels/relationships"
	"github.com/tokenized/channels/wallet"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/logger"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/wire"
	"github.com/tokenized/threads"

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
	ErrWrongMessageID      = errors.New("Wrong Message ID")

	endian = binary.LittleEndian
)

// Wallet defines the wallet functions that the Channels client needs direct access to.
type Wallet interface {
	AddMerkleProof(ctx context.Context, merkleProof *merkle_proof.MerkleProof) error
	AddTx(ctx context.Context, contextID bitcoin.Hash32, tx *wire.MsgTx) error
}

type Client struct {
	protocols *channels.Protocols

	baseKey  bitcoin.Key
	channels Channels

	// Used to create new channels
	peerChannelsAccount *peer_channels.Account

	store               storage.StreamReadWriter
	wallet              Wallet
	peerChannelsFactory *peer_channels.Factory

	messagesLock    sync.Mutex
	messagesChannel chan ChannelMessage

	channelHashes []bitcoin.Hash32 // used to load channels from storage

	accountThread  threads.Thread
	channelThreads threads.Threads

	lock sync.RWMutex
}

func SupportedProtocols() envelope.ProtocolIDs {
	return envelope.ProtocolIDs{
		channels.ProtocolIDResponse,
		channels.ProtocolIDSignedMessages,
		relationships.ProtocolID,
		invoices.ProtocolID,
		merkle_proofs.ProtocolID,
		channelsPeerChannels.ProtocolID,
	}
}

func BuildChannelsProtocols() *channels.Protocols {
	protocols := channels.NewProtocols(merkle_proofs.NewProtocol(), invoices.NewProtocol(),
		relationships.NewProtocol(), channelsPeerChannels.NewProtocol())

	return protocols
}

func NewClient(baseKey bitcoin.Key, store storage.StreamReadWriter,
	protocols *channels.Protocols, wallet Wallet,
	peerChannelsFactory *peer_channels.Factory) *Client {
	return &Client{
		protocols:           protocols,
		baseKey:             baseKey,
		store:               store,
		wallet:              wallet,
		peerChannelsFactory: peerChannelsFactory,
	}
}

func (c *Client) Protocols() *channels.Protocols {
	return c.protocols
}

func (c *Client) SetPeerChannelsAccount(account peer_channels.Account) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.peerChannelsAccount = &account
}

func (c *Client) createPeerChannelsClient(ctx context.Context) (peer_channels.Client, error) {
	if c.peerChannelsAccount == nil {
		return nil, errors.New("Missing Peer Channels Account")
	}

	return c.peerChannelsFactory.NewClient(c.peerChannelsAccount.BaseURL)
}

func (c *Client) createPeerChannelsAccountClient(ctx context.Context) (peer_channels.AccountClient, error) {
	if c.peerChannelsAccount == nil {
		return nil, errors.New("Missing Peer Channels Account")
	}

	return c.peerChannelsFactory.NewAccountClient(*c.peerChannelsAccount)
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

func (c *Client) GetChannelByHash(channelHash bitcoin.Hash32) (*Channel, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, channel := range c.channels {
		hash := channel.Hash()
		if hash.Equal(&channelHash) {
			return channel, nil
		}
	}

	return nil, nil
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
	c.lock.Lock()
	defer c.lock.Unlock()

	accountClient, err := c.createPeerChannelsAccountClient(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "create account client")
	}

	peerChannel, err := accountClient.CreatePublicChannel(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "create peer channel")
	}

	peerChannels := channels.PeerChannels{
		{
			BaseURL: accountClient.BaseURL(),
			ID:      peerChannel.ID,
		},
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.String("channel", peerChannel.ID),
	}, "Created incoming public peer channel for relationship initiation")

	hash, key := wallet.GenerateHashKey(c.baseKey, contextID)
	channel := NewChannel(ChannelTypeRelationshipInitiation, hash, key, peerChannels, c.store,
		c.peerChannelsFactory)

	c.channels = append(c.channels, channel)

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
	c.lock.Lock()
	defer c.lock.Unlock()

	accountClient, err := c.createPeerChannelsAccountClient(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "create account client")
	}

	peerChannel, err := accountClient.CreateChannel(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "create peer channel")
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.String("channel", peerChannel.ID),
	}, "Created incoming peer channel for relationship")

	peerChannels := channels.PeerChannels{
		{
			BaseURL:    accountClient.BaseURL(),
			ID:         peerChannel.ID,
			WriteToken: peerChannel.WriteToken,
		},
	}

	hash, key := wallet.GenerateHashKey(c.baseKey, contextID)
	channel := NewChannel(ChannelTypeRelationship, hash, key, peerChannels, c.store,
		c.peerChannelsFactory)

	c.channels = append(c.channels, channel)

	return channel, nil
}

func (c *Client) CreateInitialServiceChannel(ctx context.Context,
	contextID bitcoin.Hash32) (*Channel, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.peerChannelsAccount == nil {
		return nil, errors.New("Missing Peer Channels Account")
	}

	hash, key := wallet.GenerateHashKey(c.baseKey, contextID)
	publicKey := key.PublicKey()

	channelID := channelsPeerChannels.CalculatePeerChannelsServiceChannelID(publicKey)
	channelToken := channelsPeerChannels.CalculatePeerChannelsServiceChannelToken(publicKey)

	peerChannels := channels.PeerChannels{
		{
			BaseURL: c.peerChannelsAccount.BaseURL,
			ID:      channelID,
			// Only set the read token when the channel isn't part of the client's peer channel account
			ReadToken: channelToken,
		},
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.String("channel", channelID),
		logger.Stringer("public_key", key.PublicKey()),
	}, "Calculated incoming peer channel for initial service channel")

	channel := NewChannel(ChannelTypeRelationship, hash, key, peerChannels, c.store,
		c.peerChannelsFactory)

	c.channels = append(c.channels, channel)

	return channel, nil
}

// RegisterRelationshipChannel registers an existing channel with the client.
func (c *Client) RegisterRelationshipChannel(ctx context.Context,
	channelHash bitcoin.Hash32, peerChannels channels.PeerChannels) (*Channel, error) {

	channelKey, err := c.BaseKey().AddHash(channelHash)
	if err != nil {
		return nil, errors.Wrap(err, "add hash")
	}
	channel := NewChannel(ChannelTypeRelationship, channelHash, channelKey, peerChannels, c.store,
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
	var selects []reflect.SelectCase
	var wait sync.WaitGroup
	incomingMessages := make(chan peer_channels.Message, 100)
	var stopper threads.StopCombiner

	c.lock.Lock()

	var accountThread *threads.InterruptableThread
	if c.peerChannelsAccount != nil {
		logger.InfoWithFields(ctx, []logger.Field{
			logger.String("url", c.peerChannelsAccount.BaseURL),
			logger.String("account", c.peerChannelsAccount.AccountID),
		}, "Listening for messages on account")

		accountClient, err := c.createPeerChannelsAccountClient(ctx)
		if err != nil {
			c.lock.Unlock()
			return errors.Wrap(err, "create account client")
		}

		thread, complete := threads.NewInterruptableThreadComplete("Listen for Account Messages",
			func(ctx context.Context, interrupt <-chan interface{}) error {
				return accountClient.Listen(ctx, true, incomingMessages, interrupt)
			}, &wait)
		c.accountThread = thread
		accountThread = thread
		stopper.Add(accountThread)

		selects = append(selects, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(complete),
		})
	}

	var channelThreads threads.Threads
	for _, channel := range c.channels {
		incomingPeerChannels := channel.IncomingPeerChannels()
		for _, peerChannel := range incomingPeerChannels {
			if len(peerChannel.ReadToken) == 0 {
				continue
			}

			logger.InfoWithFields(ctx, []logger.Field{
				logger.String("url", peerChannel.BaseURL),
				logger.String("channel", peerChannel.ID),
			}, "Listening for messages on channel")

			client, err := c.peerChannelsFactory.NewClient(peerChannel.BaseURL)
			if err != nil {
				c.lock.Unlock()
				return errors.Wrap(err, "create peer channel client")
			}

			thread, complete := threads.NewInterruptableThreadComplete("Listen for Channel Messages",
				func(ctx context.Context, interrupt <-chan interface{}) error {
					return client.Listen(ctx, peerChannel.ReadToken, true, incomingMessages,
						interrupt)
				}, &wait)
			stopper.Add(thread)

			c.channelThreads = append(c.channelThreads, thread)
			channelThreads = append(channelThreads, thread)

			selects = append(selects, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(complete),
			})
		}
	}

	c.lock.Unlock()

	handleThread, handleThreadComplete := threads.NewUninterruptableThreadComplete("Handle Messages",
		func(ctx context.Context) error {
			return c.handleMessages(ctx, incomingMessages)
		}, &wait)
	handleSelectIndex := len(selects)
	selects = append(selects, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(handleThreadComplete),
	})

	selects = append(selects, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(interrupt),
	})

	if accountThread != nil {
		accountThread.Start(ctx)
	}
	for _, channelThread := range channelThreads {
		channelThread.Start(ctx)
	}
	handleThread.Start(ctx)

	index, selectValue, valueReceived := reflect.Select(selects)
	var selectErr, listenErr error
	if valueReceived {
		selectInterface := selectValue.Interface()
		if selectInterface != nil {
			err, ok := selectInterface.(error)
			if ok {
				selectErr = err
			}
		}
	}

	if index == handleSelectIndex {
		logger.Warn(ctx, "Handle messages thread stopped : %s", selectErr)
	} else if index < handleSelectIndex {
		logger.Warn(ctx, "One of the listen threads stopped : %s", selectErr)
		listenErr = selectErr
	}

	close(incomingMessages)
	stopper.Stop(ctx)
	wait.Wait()
	return listenErr
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
	ctx = logger.ContextWithLogFields(ctx, logger.String("channel", message.ChannelID),
		logger.Uint64("sequence", message.Sequence))

	if message.ContentType != peer_channels.ContentTypeBinary {
		logger.WarnWithFields(ctx, []logger.Field{
			logger.String("content_type", message.ContentType),
		}, "Message content not binary")
		return nil
	}

	logger.VerboseWithFields(ctx, []logger.Field{
		logger.Stringer("message_hash", message.Hash()),
	}, "Received message")

	channel, err := c.GetChannel(message.ChannelID)
	if err != nil {
		return errors.Wrap(err, "get channel")
	}
	if channel == nil {
		logger.Warn(ctx, "Unknown channel")
		return nil
	}

	ctx = logger.ContextWithLogFields(ctx, logger.String("channel_id", message.ChannelID),
		logger.Stringer("channel_hash", channel.Hash()))

	if err := c.processMessage(ctx, channel, message); err != nil {
		logger.Warn(ctx, "Process message : %s", err)
		return nil
	}

	peerChannel := channel.GetIncomingPeerChannel(message.ChannelID)
	if peerChannel == nil {
		return errors.New("Peer Channel Not Found")
	}

	if len(peerChannel.ReadToken) > 0 {
		client, err := c.peerChannelsFactory.NewClient(peerChannel.BaseURL)
		if err != nil {
			return errors.Wrap(err, "create peer channels client")
		}

		if err := client.MarkMessages(ctx, peerChannel.ID, peerChannel.ReadToken, message.Sequence,
			true, true); err != nil {
			return errors.Wrap(err, "mark message read channel")
		}

		logger.Info(ctx, "Marked message as read with channel")
	} else {
		c.lock.Lock()

		// Assume the channel is under the account
		if c.peerChannelsAccount == nil || len(c.peerChannelsAccount.Token) == 0 {
			c.lock.Unlock()
			return errors.New("No account or token to mark message as read")
		}

		client, err := c.createPeerChannelsClient(ctx)
		if err != nil {
			c.lock.Unlock()
			return errors.Wrap(err, "create peer channels client")
		}

		if err := client.MarkMessages(ctx, peerChannel.ID, c.peerChannelsAccount.Token,
			message.Sequence, true, true); err != nil {
			c.lock.Unlock()
			return errors.Wrap(err, "mark message read account")
		}

		c.lock.Unlock()

		logger.Info(ctx, "Marked message as read with account")
	}

	return nil
}

func (c *Client) processMessage(ctx context.Context, channel *Channel,
	message *peer_channels.Message) error {

	result, err := channel.ProcessMessage(ctx, c.protocols, c.wallet, message)
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
	c.lock.RLock()
	defer c.lock.RUnlock()

	publicKey := c.baseKey.PublicKey()
	ctx = logger.ContextWithLogFields(ctx, logger.Stringer("client", publicKey))
	path := fmt.Sprintf("%s/%s", clientsPath, publicKey)

	if err := storage.StreamWrite(ctx, c.store, path, c); err != nil {
		return errors.Wrap(err, "write")
	}
	logger.Info(ctx, "Saved channels client")

	for i, channel := range c.channels {
		channelCtx := logger.ContextWithLogFields(ctx,
			logger.Stringer("channel_hash", channel.Hash()))
		if err := channel.Save(channelCtx); err != nil {
			return errors.Wrapf(err, "channel %d: %s", i, channel.Hash())
		}
		logger.Info(channelCtx, "Saved channel")
	}

	return nil
}

func (c *Client) Load(ctx context.Context) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	path := fmt.Sprintf("%s/%s", clientsPath, c.baseKey.PublicKey())

	if err := storage.StreamRead(ctx, c.store, path, c); err != nil {
		if errors.Cause(err) == storage.ErrNotFound {
			return nil
		}
		return errors.Wrap(err, "read")
	}

	// Load channels
	c.channels = make(Channels, len(c.channelHashes))
	for i, channelHash := range c.channelHashes {
		channelKey, err := c.baseKey.AddHash(channelHash)
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
