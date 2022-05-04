package client

import (
	"bytes"
	"context"
	"sync"

	"github.com/tokenized/channels"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
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
)

type Client struct {
	account             Account
	identity            channels.Identity
	messageHandler      HandleMessage
	peerChannelsFactory *peer_channels.Factory
	incomingMessages    chan peer_channels.Message

	channels Channels

	lock sync.RWMutex
}

type Account struct {
	BaseURL string `bsor:"1" json:"base_url"`
	ID      string `bsor:"2" json:"id"`
	Token   string `bsor:"3" json:"token"`
}

type HandleMessage func(ctx context.Context, channel *Channel, sequence uint32,
	protocolIDs envelope.ProtocolIDs, payload bitcoin.ScriptItems) error

func NewClient(account Account, identity channels.Identity, handleMessage HandleMessage,
	peerChannelsFactory *peer_channels.Factory) *Client {
	return &Client{
		account:             account,
		identity:            identity,
		messageHandler:      handleMessage,
		peerChannelsFactory: peerChannelsFactory,
	}
}

func (c *Client) AddChannel(channel *Channel) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.channels = append(c.channels, channel)
	return nil
}

func (c *Client) GetChannel(channelID string) (*Channel, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, channel := range c.channels {
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
	for i, channel := range c.channels {
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

	peerClient, err := c.peerChannelsFactory.NewClient(c.account.BaseURL)
	if err != nil {
		return errors.Wrap(err, "peer client")
	}

	listenThread := threads.NewThread("Listen for Messages", func(ctx context.Context,
		interrupt <-chan interface{}) error {
		return peerClient.AccountListen(ctx, c.account.ID, c.account.Token, c.incomingMessages,
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
		if err := c.handleMessage(ctx, message); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) handleMessage(ctx context.Context, message peer_channels.Message) error {
	logger.VerboseWithFields(ctx, []logger.Field{
		logger.String("channel", message.ChannelID),
		logger.String("content_type", message.ContentType),
		logger.Uint32("sequence", message.Sequence),
		logger.Stringer("received", message.Received),
	}, "Received message")

	if message.ContentType != peer_channels.ContentTypeBinary {
		logger.WarnWithFields(ctx, []logger.Field{
			logger.String("channel", message.ChannelID),
			logger.String("content_type", message.ContentType),
			logger.Uint32("sequence", message.Sequence),
			logger.Stringer("received", message.Received),
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
		}, "Unknown channel")
	}

	if err := c.processMessage(ctx, channel, message); err != nil {
		logger.WarnWithFields(ctx, []logger.Field{
			logger.String("channel", message.ChannelID),
			logger.Uint32("sequence", message.Sequence),
			logger.Stringer("received", message.Received),
		}, "Process message : %s", err)
		return nil
	}

	// if err := c.messageHandler(ctx, channel, message.Sequence, protocolIDs, payload); err != nil {
	// 	return errors.Wrap(err, "handle")
	// }

	return nil
}

func (c *Client) processMessage(ctx context.Context, channel *Channel,
	message peer_channels.Message) error {

	protocolIDs, payload, err := envelopeV1.Parse(bytes.NewReader(message.Payload))
	if err != nil {
		return errors.Wrap(err, "parse envelope")
	}

	if len(protocolIDs) == 0 {
		return errors.New("No Protocol IDs")
	}

	if err := channel.ProcessMessage(ctx, message, protocolIDs, payload); err != nil {
		return errors.Wrap(err, "channel")
	}

	return nil
}
