package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/tokenized/channels"
	"github.com/tokenized/logger"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"

	"github.com/pkg/errors"
)

const (
	communicationChannelsVersion = uint8(0)
)

type CommunicationChannel struct {
	peerChannels channels.PeerChannels

	lowestUnprocessed uint32
	loadedOffset      int
	savedOffset       uint32
	messageCount      uint32
	sequencedMessages Messages

	basePath string
	store    storage.StreamReadWriter

	lock sync.RWMutex
}

func NewCommunicationChannel(peerChannels channels.PeerChannels,
	store storage.StreamReadWriter, basePath string) *CommunicationChannel {
	return &CommunicationChannel{
		peerChannels: peerChannels,
		store:        store,
		basePath:     basePath,
	}
}

func newCommunicationChannel(store storage.StreamReadWriter,
	basePath string) *CommunicationChannel {
	return &CommunicationChannel{
		store:    store,
		basePath: basePath,
	}
}

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

// GetMessage returns the message for the specified id. It returns a copy so the message
// modification functions will not work.
func (c *CommunicationChannel) GetMessage(ctx context.Context, id uint64) (*Message, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	message, err := c.getMessage(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "get message")
	}

	msg := *message
	return &msg, nil
}

func (c *CommunicationChannel) GetUnprocessedMessages(ctx context.Context) (Messages, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	var result Messages
	for _, message := range c.sequencedMessages {
		if message.IsProcessed() {
			continue
		}

		msg := *message
		result = append(result, &msg)
	}

	return result, nil
}

func (c *CommunicationChannel) MarkMessageIsProcessed(ctx context.Context, id uint64) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	message, err := c.getMessage(ctx, id)
	if err != nil {
		return errors.Wrap(err, "get message")
	}

	message.setIsProcessed()

	if err := c.updateLowestUnprocessed(ctx, id); err != nil {
		return errors.Wrap(err, "update lowest unprocessed")
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Uint64("message_id", id),
	}, "Marked message is processed")

	return nil
}

func (c *CommunicationChannel) SetMessageIsAwaitingResponse(ctx context.Context, id uint64) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	message, err := c.getMessage(ctx, id)
	if err != nil {
		return errors.Wrap(err, "get message")
	}

	message.setIsAwaitingResponse()

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Uint64("message_id", id),
	}, "Set message is awaiting response")

	if err := c.updateLowestUnprocessed(ctx, id); err != nil {
		return errors.Wrap(err, "update lowest unprocessed")
	}

	return nil
}

func (c *CommunicationChannel) ClearMessageIsAwaitingResponse(ctx context.Context, id uint64) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	message, err := c.getMessage(ctx, id)
	if err != nil {
		return errors.Wrap(err, "get message")
	}

	message.clearIsAwaitingResponse()

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Uint64("message_id", id),
	}, "Clear message is awaiting response")

	if err := c.updateLowestUnprocessed(ctx, id); err != nil {
		return errors.Wrap(err, "update lowest unprocessed")
	}

	return nil
}

func (c *CommunicationChannel) updateLowestUnprocessed(ctx context.Context, id uint64) error {
	offset := id - uint64(c.loadedOffset)
	current := uint32(id)
	count := uint64(len(c.sequencedMessages))

	if current != c.lowestUnprocessed {
		return nil
	}

	for {
		message := c.sequencedMessages[offset]
		if !message.IsProcessed() || message.IsAwaitingResponse() {
			return nil
		}

		current++
		c.lowestUnprocessed++
		offset++

		if offset == count {
			return nil
		}
	}
}

func (c *CommunicationChannel) getMessage(ctx context.Context, id uint64) (*Message, error) {
	if id < uint64(c.loadedOffset) {
		sequencedMessages, err := loadSequencedMessageFile(ctx, c.store, c.basePath, int(id))
		if err != nil {
			return nil, errors.Wrapf(err, "load file %d", id)
		}

		offset := id - sequencedMessages[0].ID()

		if err := saveSequencedMessageFile(ctx, c.store, c.basePath, sequencedMessages); err != nil {
			return nil, errors.Wrapf(err, "save file %d", id)
		}

		return sequencedMessages[offset], nil
	}

	offset := uint32(id - uint64(c.loadedOffset))
	if offset >= uint32(len(c.sequencedMessages)) {
		return nil, ErrMessageNotFound
	}
	return c.sequencedMessages[offset], nil
}

func (c *CommunicationChannel) AddUnsequencedMessage(ctx context.Context, payload bitcoin.Script,
	wrap *channels.WrappedMessage) (*Message, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	nextID := uint64(c.loadedOffset + len(c.sequencedMessages))
	msg := &Message{
		id:        nextID,
		payload:   payload,
		timestamp: channels.Now(),
	}
	c.sequencedMessages = append(c.sequencedMessages, msg)

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Uint64("message_id", msg.ID()),
		logger.Int("bytes", len(payload)),
	}, "New message")

	return msg, nil
}

func (c *CommunicationChannel) AddMessage(ctx context.Context, payload bitcoin.Script,
	wrap *channels.WrappedMessage) (*Message, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if wrap.MessageID == nil {
		return nil, errors.New("Unsequenced Messages Not Supported")
	}

	nextID := uint64(c.loadedOffset + len(c.sequencedMessages))
	if wrap.MessageID.MessageID != nextID {
		return nil, errors.Wrapf(ErrWrongMessageID, "got %d, want %d", wrap.MessageID.MessageID,
			nextID)
	}

	msg := &Message{
		id:        nextID,
		payload:   payload,
		timestamp: channels.Now(),
	}
	c.sequencedMessages = append(c.sequencedMessages, msg)

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Uint64("message_id", msg.ID()),
		logger.Int("bytes", len(payload)),
	}, "New sequenced message")

	return msg, nil
}

func (c *CommunicationChannel) newMessage(ctx context.Context) (*Message, error) {
	msg := &Message{
		id: uint64(c.loadedOffset + len(c.sequencedMessages)),
	}

	c.sequencedMessages = append(c.sequencedMessages, msg)

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Uint64("message_id", msg.ID()),
	}, "New message")
	return msg, nil
}

func (c *CommunicationChannel) sendMessage(ctx context.Context,
	peerChannelsFactory *peer_channels.Factory, message *Message) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if err := sendMessage(ctx, peerChannelsFactory, c.peerChannels, message.Payload()); err != nil {
		return errors.Wrap(err, "send")
	}

	message.SetNow()
	return nil
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

		if err := peerClient.WriteMessage(ctx, peerChannel.ID, peerChannel.WriteToken,
			peer_channels.ContentTypeBinary, bytes.NewReader(message)); err != nil {
			logger.WarnWithFields(ctx, []logger.Field{
				logger.String("base_url", peerChannel.BaseURL),
				logger.String("channel", peerChannel.ID),
			}, "Failed to post peer channel message : %s", err)
			lastErr = err
		} else {
			logger.InfoWithFields(ctx, []logger.Field{
				logger.String("base_url", peerChannel.BaseURL),
				logger.String("channel", peerChannel.ID),
			}, "Posted peer channel message")
			success = true
		}
	}

	if !success {
		return lastErr
	}

	return nil
}

func (c *CommunicationChannel) Save(ctx context.Context) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	path := fmt.Sprintf("%s/channel", c.basePath)
	if err := storage.StreamWrite(ctx, c.store, path, c); err != nil {
		return errors.Wrap(err, "write")
	}

	if err := c.saveSequencedMessages(ctx); err != nil {
		return errors.Wrap(err, "sequenced messages")
	}

	return nil
}

func (c *CommunicationChannel) saveSequencedMessages(ctx context.Context) error {
	offset := int(c.savedOffset)
	mod := offset % messagesPerFile
	if mod != 0 {
		offset -= mod
	}
	endOffset := c.loadedOffset + len(c.sequencedMessages)

	for ; offset < endOffset; offset += messagesPerFile {
		path := fmt.Sprintf("%s/sequenced/%08x", c.basePath, offset/messagesPerFile)

		start := offset - c.loadedOffset
		end := start + messagesPerFile
		if end >= endOffset {
			end = endOffset
		}

		if err := storage.StreamWrite(ctx, c.store, path, c.sequencedMessages[start:end]); err != nil {
			return errors.Wrapf(err, "write %d-%d", start, end)
		}
	}

	c.savedOffset = uint32(endOffset)
	return nil
}

func saveSequencedMessageFile(ctx context.Context, store storage.StreamWriter, basePath string,
	sequencedMessages Messages) error {

	if len(sequencedMessages) == 0 {
		return errors.New("Messages empty")
	}

	offset := int(sequencedMessages[0].ID())
	if offset%messagesPerFile != 0 {
		return fmt.Errorf("Messages don't start at start of file: %d", offset)
	}

	path := fmt.Sprintf("%s/sequenced/%08x", basePath, offset/messagesPerFile)

	if err := storage.StreamWrite(ctx, store, path, sequencedMessages); err != nil {
		return errors.Wrapf(err, "write %d-%d", offset, offset+len(sequencedMessages)-1)
	}

	return nil
}

func (c *CommunicationChannel) Load(ctx context.Context) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	path := fmt.Sprintf("%s/channel", c.basePath)
	if err := storage.StreamRead(ctx, c.store, path, c); err != nil {
		return errors.Wrap(err, "read")
	}

	offset := 0
	if c.messageCount > messagesPerFile {
		offset = int(c.messageCount) - messagesPerFile
	}

	if offset > int(c.lowestUnprocessed) {
		offset = int(c.lowestUnprocessed)
	}
	if err := c.loadSequencedMessages(ctx, offset); err != nil {
		return errors.Wrap(err, "sequenced messages")
	}

	return nil
}

func (c *CommunicationChannel) loadSequencedMessages(ctx context.Context, offset int) error {
	mod := offset % messagesPerFile
	if mod != 0 {
		offset -= mod
	}

	mod = offset % messagesPerFile
	if mod != 0 {
		offset -= mod
	}

	loadOffset := offset

	c.sequencedMessages = make(Messages, 0, messagesPerFile)
	for {
		path := fmt.Sprintf("%s/sequenced/%08x", c.basePath, offset/messagesPerFile)

		var newMessages Messages
		if err := storage.StreamRead(ctx, c.store, path, &newMessages); err != nil {
			if errors.Cause(err) == storage.ErrNotFound {
				break
			}
			return errors.Wrapf(err, "read %d-", offset)
		}

		for _, message := range newMessages {
			message.id = uint64(offset)
			c.sequencedMessages = append(c.sequencedMessages, message)
			offset++
		}

		if len(newMessages) != messagesPerFile {
			// file not full
			break
		}
	}

	c.loadedOffset = loadOffset
	return nil
}

func loadSequencedMessageFile(ctx context.Context, store storage.StreamReader, basePath string,
	offset int) (Messages, error) {

	mod := offset % messagesPerFile
	if mod != 0 {
		offset -= mod
	}

	sequencedMessages := make(Messages, 0, messagesPerFile)
	path := fmt.Sprintf("%s/sequenced/%08x", basePath, offset/messagesPerFile)

	var newMessages Messages
	if err := storage.StreamRead(ctx, store, path, &newMessages); err != nil {
		if errors.Cause(err) == storage.ErrNotFound {
			return nil, err
		}
		return nil, errors.Wrapf(err, "read %d-", offset)
	}

	for _, message := range newMessages {
		message.id = uint64(offset)
		sequencedMessages = append(sequencedMessages, message)
		offset++
	}

	return sequencedMessages, nil
}

func (c *CommunicationChannel) Serialize(w io.Writer) error {
	if err := binary.Write(w, endian, communicationChannelsVersion); err != nil {
		return errors.Wrap(err, "version")
	}

	peerChannelsBSOR, err := bsor.MarshalBinary(c.peerChannels)
	if err != nil {
		return errors.Wrap(err, "marshal peer channels")
	}

	if err := binary.Write(w, endian, uint32(len(peerChannelsBSOR))); err != nil {
		return errors.Wrap(err, "peer channels size")
	}

	if _, err := w.Write(peerChannelsBSOR); err != nil {
		return errors.Wrap(err, "write peer channels")
	}

	c.messageCount = uint32(c.loadedOffset + len(c.sequencedMessages))
	if err := binary.Write(w, endian, c.messageCount); err != nil {
		return errors.Wrap(err, "message count")
	}

	if err := binary.Write(w, endian, c.lowestUnprocessed); err != nil {
		return errors.Wrap(err, "lowest unprocessed")
	}

	return nil
}

func (c *CommunicationChannel) Deserialize(r io.Reader) error {
	var version uint8
	if err := binary.Read(r, endian, &version); err != nil {
		return errors.Wrap(err, "version")
	}
	if version != 0 {
		return errors.New("Unsupported version")
	}

	var peerChannelsSize uint32
	if err := binary.Read(r, endian, &peerChannelsSize); err != nil {
		return errors.Wrap(err, "peer channels size")
	}

	peerChannelsBSOR := make([]byte, peerChannelsSize)
	if _, err := io.ReadFull(r, peerChannelsBSOR); err != nil {
		return errors.Wrap(err, "read peer channels")
	}

	var peerChannels channels.PeerChannels
	if _, err := bsor.UnmarshalBinary(peerChannelsBSOR, &peerChannels); err != nil {
		return errors.Wrap(err, "unmarshal peer channels")
	}
	c.peerChannels = peerChannels

	if err := binary.Read(r, endian, &c.messageCount); err != nil {
		return errors.Wrap(err, "message count")
	}

	if err := binary.Read(r, endian, &c.lowestUnprocessed); err != nil {
		return errors.Wrap(err, "lowest unprocessed")
	}

	return nil
}
