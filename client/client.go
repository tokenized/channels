package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/wallet"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/threads"

	"github.com/pkg/errors"
)

var (
	ErrSignatureRequired   = errors.New("Signature Required")
	ErrMissingRelationship = errors.New("Missing Relationship")
	ErrWrongPublicKey      = errors.New("Wrong Public Key")
	ErrMessageNotFound     = errors.New("Message Not Found")
)

type Client struct {
	baseKey  bitcoin.Key
	account  Account
	channels Channels

	peerChannelsFactory *peer_channels.Factory

	lock sync.RWMutex
}

type Account struct {
	BaseURL string `bsor:"1" json:"base_url"`
	ID      string `bsor:"2" json:"id"`
	Token   string `bsor:"3" json:"token"`
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

func NewClient(baseKey bitcoin.Key, account Account,
	peerChannelsFactory *peer_channels.Factory) *Client {
	return &Client{
		baseKey:             baseKey,
		account:             account,
		peerChannelsFactory: peerChannelsFactory,
	}
}

func (c *Client) BaseKey() bitcoin.Key {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.baseKey
}

func (c *Client) ChannelKey(channel *Channel) bitcoin.Key {
	key, err := c.BaseKey().AddHash(channel.Hash())
	if err != nil {
		// This can only happen if the hash creates an out of range key which should have been
		// checked already.
		panic(fmt.Sprintf("Failed to add hash to key : %s", err))
	}

	return key
}

func (c *Client) Account() Account {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.account
}

func (c *Client) Channel(index int) *Channel {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.channels[index]
}

func GetWriteToken(peerChannel *peer_channels.Channel) string {
	for _, token := range peerChannel.AccessTokens {
		if token.CanWrite {
			return token.Token
		}
	}

	return ""
}

// CreatePublicChannel creates a new channel to share publicly so other users can initiation
// relationships. Those relationships will be
func (c *Client) CreatePublicChannel(ctx context.Context) (*Channel, error) {
	account := c.Account()
	peerClient, err := c.peerChannelsFactory.NewClient(account.BaseURL)
	if err != nil {
		return nil, errors.Wrap(err, "peer channel client")
	}

	peerChannel, err := peerClient.CreatePublicChannel(ctx, account.ID, account.Token)
	if err != nil {
		return nil, errors.Wrap(err, "create peer channel")
	}

	peerChannels := channels.PeerChannels{
		{
			BaseURL: peer_channels.MockClientURL,
			ID:      peerChannel.ID,
		},
	}

	hash, _ := wallet.GenerateHashKey(c.BaseKey(), "test")
	channel := NewPublicChannel(c.peerChannelsFactory, hash, peerChannels)

	c.lock.Lock()
	c.channels = append(c.channels, channel)
	c.lock.Unlock()

	return channel, nil
}

func (c *Client) CreatePrivateChannel(ctx context.Context) (*Channel, error) {
	account := c.Account()
	peerClient, err := c.peerChannelsFactory.NewClient(account.BaseURL)
	if err != nil {
		return nil, errors.Wrap(err, "peer channel client")
	}

	peerChannel, err := peerClient.CreateChannel(ctx, account.ID, account.Token)
	if err != nil {
		return nil, errors.Wrap(err, "create peer channel")
	}

	peerChannels := channels.PeerChannels{
		{
			BaseURL:    peer_channels.MockClientURL,
			ID:         peerChannel.ID,
			WriteToken: GetWriteToken(peerChannel),
		},
	}

	hash, _ := wallet.GenerateHashKey(c.BaseKey(), "test")
	channel := NewPrivateChannel(c.peerChannelsFactory, hash, peerChannels)

	c.lock.Lock()
	c.channels = append(c.channels, channel)
	c.lock.Unlock()

	return channel, nil
}

func (c *Client) GetChannel(channelID string) (*Channel, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, channel := range c.channels {
		if channel.internal.HasPeerChannelID(channelID) {
			return channel, nil
		}
	}

	return nil, nil
}

func (c *Client) GetUnprocessedMessages(ctx context.Context) (ChannelMessages, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	var result ChannelMessages
	for i, channel := range c.channels {
		messages, err := channel.internal.GetUnprocessedMessages(ctx)
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

func (c *Client) Run(ctx context.Context, interrupt <-chan interface{}) error {
	wait := &sync.WaitGroup{}
	incomingMessages := make(chan peer_channels.Message)
	account := c.Account()

	peerClient, err := c.peerChannelsFactory.NewClient(account.BaseURL)
	if err != nil {
		return errors.Wrap(err, "peer client")
	}

	listenThread := threads.NewThread("Listen for Messages", func(ctx context.Context,
		interrupt <-chan interface{}) error {
		return peerClient.AccountListen(ctx, account.ID, account.Token, incomingMessages, interrupt)
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

	if err := channel.ProcessMessage(ctx, message); err != nil {
		return errors.Wrap(err, "channel")
	}

	return nil
}
