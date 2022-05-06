package client

import (
	"context"
	"sync"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/threads"

	"github.com/pkg/errors"
)

var (
	ErrSignatureRequired   = errors.New("Signature Required")
	ErrMissingRelationship = errors.New("Missing Relationship")
	ErrWrongPublicKey      = errors.New("Wrong Public Key")
)

type Client struct {
	Account  Account
	Identity channels.Identity

	peerChannelsFactory *peer_channels.Factory
	incomingMessages    chan peer_channels.Message

	Channels Channels

	lock sync.RWMutex
}

type Account struct {
	BaseURL string `bsor:"1" json:"base_url"`
	ID      string `bsor:"2" json:"id"`
	Token   string `bsor:"3" json:"token"`
}

func NewClient(account Account, identity channels.Identity,
	peerChannelsFactory *peer_channels.Factory) *Client {
	return &Client{
		Account:             account,
		Identity:            identity,
		peerChannelsFactory: peerChannelsFactory,
	}
}

func (c *Client) AddChannel(channel *Channel) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.Channels = append(c.Channels, channel)
	return nil
}

func (c *Client) GetChannel(channelID string) (*Channel, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, channel := range c.Channels {
		if channel.Incoming.HasPeerChannelID(channelID) {
			return channel, nil
		}
	}

	return nil, nil
}

func (c *Client) GetUnprocessedMessages(ctx context.Context) (ChannelMessages, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	var result ChannelMessages
	for i, channel := range c.Channels {
		messages, err := channel.Incoming.GetUnprocessedMessages(ctx)
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
	c.incomingMessages = make(chan peer_channels.Message)

	peerClient, err := c.peerChannelsFactory.NewClient(c.Account.BaseURL)
	if err != nil {
		return errors.Wrap(err, "peer client")
	}

	listenThread := threads.NewThread("Listen for Messages", func(ctx context.Context,
		interrupt <-chan interface{}) error {
		return peerClient.AccountListen(ctx, c.Account.ID, c.Account.Token, c.incomingMessages,
			interrupt)
	})
	listenThread.SetWait(wait)
	listenThreadComplete := listenThread.GetCompleteChannel()

	handleThread := threads.NewThreadWithoutStop("Handle Messages", c.handleMessages)
	handleThread.SetWait(wait)
	handleThreadComplete := handleThread.GetCompleteChannel()

	listenThread.Start(ctx)
	handleThread.Start(ctx)

	select {
	case <-interrupt:
		listenThread.Stop(ctx)
		close(c.incomingMessages)

	case <-listenThreadComplete:
		logger.Warn(ctx, "Listen for messages thread stopped : %s", listenThread.Error())
		listenThread.Stop(ctx)
		close(c.incomingMessages)

	case <-handleThreadComplete:
		logger.Warn(ctx, "Handle messages thread stopped : %s", handleThread.Error())
		listenThread.Stop(ctx)
	}

	wait.Wait()
	return listenThread.Error()
}

func (c *Client) handleMessages(ctx context.Context) error {
	for message := range c.incomingMessages {
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
