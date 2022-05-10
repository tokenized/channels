package client

import (
	"context"
	"sync"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"

	"github.com/pkg/errors"
)

type CommunicationChannel struct {
	peerChannels channels.PeerChannels

	messages   Messages
	messageMap map[bitcoin.Hash32]int

	lock sync.RWMutex
}

func NewCommunicationChannel(peerChannels channels.PeerChannels) *CommunicationChannel {
	return &CommunicationChannel{
		peerChannels: peerChannels,
		messageMap:   make(map[bitcoin.Hash32]int),
	}
}

// func NewEmptyCommunicationChannel() *CommunicationChannel {
// 	return &CommunicationChannel{
// 		messageMap: make(map[bitcoin.Hash32]int),
// 	}
// }

func (c *CommunicationChannel) HasPeerChannelID(id string) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, peerChannel := range c.peerChannels {
		if peerChannel.ID == id {
			return true
		}
	}

	return false
}

func (c *CommunicationChannel) PeerChannels() channels.PeerChannels {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.peerChannels
}

func (c *CommunicationChannel) SetPeerChannels(peerChannels channels.PeerChannels) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.peerChannels = peerChannels
	return nil
}

func (c *CommunicationChannel) GetUnprocessedMessages(ctx context.Context) (Messages, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	var result Messages
	for _, message := range c.messages {
		if message.IsProcessed {
			continue
		}

		msg := *message
		result = append(result, &msg)
	}

	return result, nil
}

func (c *CommunicationChannel) MarkMessageProcessed(ctx context.Context,
	hash bitcoin.Hash32) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	index, exists := c.messageMap[hash]
	if !exists {
		return ErrMessageNotFound
	}

	c.messages[index].IsProcessed = true
	return nil
}

func (c *CommunicationChannel) AddMessage(ctx context.Context, message bitcoin.Script) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.addMessage(ctx, message)
}

func (c *CommunicationChannel) addMessage(ctx context.Context, message bitcoin.Script) error {
	now := channels.Now()
	msg := &Message{
		Payload:  message,
		Received: &now,
	}
	hash := msg.Hash()

	c.messageMap[hash] = len(c.messages)
	c.messages = append(c.messages, msg)

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("message_hash", hash),
	}, "Added message")
	return nil
}

func (c *CommunicationChannel) sendMessage(ctx context.Context,
	peerChannelsFactory *peer_channels.Factory, message bitcoin.Script) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if err := sendMessage(ctx, peerChannelsFactory, c.peerChannels, message); err != nil {
		return errors.Wrap(err, "send")
	}

	return c.addMessage(ctx, message)
}

func sendMessage(ctx context.Context, peerChannelsFactory *peer_channels.Factory,
	peerChannels channels.PeerChannels, message bitcoin.Script) error {

	success := false
	var lastErr error
	for _, peerChannel := range peerChannels {
		peerClient, err := peerChannelsFactory.NewClient(peerChannel.BaseURL)
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
