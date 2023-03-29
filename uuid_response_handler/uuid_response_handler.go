package uuid_response_handler

import (
	"context"
	"time"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/peer_channels_listener"
	"github.com/tokenized/logger"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/peer_channels"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var (
	ErrTimeout = errors.New("Timeout")
)

// type AddUpdate func(update *messageHandler)

// Handler handles responses with UUIDs from peer channels. It expects only one response for each
// UUID and will ignore any further responses.
type Handler struct {
	handlers  map[string]map[uuid.UUID]*messageHandler
	addUpdate peer_channels_listener.AddUpdate[*messageHandler]
}

type messageHandler struct {
	channelID string
	id        uuid.UUID
	response  chan peer_channels.Message
}

func NewHandler() *Handler {
	return &Handler{
		handlers: make(map[string]map[uuid.UUID]*messageHandler),
	}
}

func (h *Handler) SetAddUpdate(addUpdate peer_channels_listener.AddUpdate[*messageHandler]) {
	h.addUpdate = addUpdate
}

// RegisterForResponse registers for a response on the specified channel containing the specified
// UUID and returns a channel the will have the first message that matches that criteria written to
// it.
func (h *Handler) RegisterForResponse(channelID string, id uuid.UUID) <-chan peer_channels.Message {
	handler := newMessageHandler(channelID, id)
	h.addUpdate(handler)
	return handler.response
}

func newMessageHandler(channelID string, id uuid.UUID) *messageHandler {
	return &messageHandler{
		channelID: channelID,
		id:        id,
		response:  make(chan peer_channels.Message, 1), // buffered to prevent locking
	}
}

func WaitWithTimeout(responseChannel <-chan peer_channels.Message,
	timeout time.Duration) (*peer_channels.Message, error) {

	select {
	case response := <-responseChannel:
		return &response, nil
	case <-time.After(timeout):
		return nil, errors.Wrap(ErrTimeout, timeout.String())
	}
}

func (h *Handler) HandleMessage(ctx context.Context, msg peer_channels.Message) error {
	id := parseUUID(bitcoin.Script(msg.Payload))
	if id == nil {
		logger.InfoWithFields(ctx, []logger.Field{
			logger.String("channel_id", msg.ChannelID),
			logger.Uint64("sequence", msg.Sequence),
		}, "Message is missing ID")
		return errors.Wrap(peer_channels_listener.MessageNotRelevent, "missing id")
	}

	channelHandlers, channelExists := h.handlers[msg.ChannelID]
	if !channelExists {
		logger.InfoWithFields(ctx, []logger.Field{
			logger.String("channel_id", msg.ChannelID),
			logger.Uint64("sequence", msg.Sequence),
		}, "No handlers found for channel")
		return errors.Wrap(peer_channels_listener.MessageNotRelevent, "channel")
	}

	handler, idExists := channelHandlers[*id]
	if !idExists {
		logger.InfoWithFields(ctx, []logger.Field{
			logger.Stringer("id", *id),
			logger.String("channel_id", msg.ChannelID),
			logger.Uint64("sequence", msg.Sequence),
		}, "No handler found for ID")
		return errors.Wrapf(peer_channels_listener.MessageNotRelevent, "id: %s", *id)
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("id", *id),
		logger.String("channel_id", msg.ChannelID),
		logger.Uint64("sequence", msg.Sequence),
	}, "Received UUID message")
	delete(channelHandlers, *id)
	if len(channelHandlers) == 0 {
		delete(h.handlers, msg.ChannelID)
	}
	response := handler.response

	response <- msg
	return nil
}

func (h *Handler) HandleUpdate(ctx context.Context, handler *messageHandler) error {
	channelHandlers, exists := h.handlers[handler.channelID]
	if !exists {
		channelHandlers = make(map[uuid.UUID]*messageHandler)
		h.handlers[handler.channelID] = channelHandlers
	}

	channelHandlers[handler.id] = handler
	return nil
}

func parseUUID(script bitcoin.Script) *uuid.UUID {
	protocols := channels.NewProtocols(channels.NewSignedProtocol(), channels.NewUUIDProtocol(),
		channels.NewReplyToProtocol(), channels.NewResponseProtocol())

	_, wrappers, err := protocols.Parse(script)
	if err != nil && errors.Cause(err) != channels.ErrUnsupportedProtocol {
		return nil
	}

	for _, wrapper := range wrappers {
		if id, ok := wrapper.(*channels.UUID); ok {
			uuid := uuid.UUID(*id)
			return &uuid
		}
	}

	return nil
}
