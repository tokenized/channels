package peer_channels_listener

import (
	"context"
	"sync"
	"time"

	"github.com/tokenized/logger"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/threads"

	"github.com/pkg/errors"
)

var (
	MessageNotRelevent = errors.New("Message Not Relevant")
)

// HandleMessage handles a peer channel message. It returns MessageNotRelevent, which can be
// wrapped, if it does not recognize the message. This will always be called in the same thread.
type HandleMessage func(ctx context.Context, msg peer_channels.Message) error

// HandleUpdate handles a struct that updates the state of a message handler. It updates it in the
// same thread that is handling messages so there is no multi-thread locking required.
type HandleUpdate[Update any] func(ctx context.Context, update Update) error

// AddUpdate adds an update struct to be handled in the same thread as the message handler. This
// function interface can be used by the message handler so that there isn't a circular dependency
// between the message handler and the listener.
type AddUpdate[Update any] func(update Update)

type PeerChannelsListener[Update any] struct {
	peerChannelsClient peer_channels.Client
	readToken          string
	handleMessage      HandleMessage
	handleUpdate       HandleUpdate[Update]
	messagesChannel    chan peer_channels.Message
	updatesChannel     chan Update
}

func NewPeerChannelsListener[Update any](peerChannelsClient peer_channels.Client, readToken string,
	channelSize int, handleMessage HandleMessage,
	handleUpdate HandleUpdate[Update]) *PeerChannelsListener[Update] {
	return &PeerChannelsListener[Update]{
		peerChannelsClient: peerChannelsClient,
		readToken:          readToken,
		handleMessage:      handleMessage,
		handleUpdate:       handleUpdate,
		messagesChannel:    make(chan peer_channels.Message, channelSize),
		updatesChannel:     make(chan Update, channelSize),
	}
}

func (l *PeerChannelsListener[Update]) AddUpdate(update Update) {
	l.updatesChannel <- update
}

func (l *PeerChannelsListener[Update]) Run(ctx context.Context,
	interrupt <-chan interface{}) error {
	var wait sync.WaitGroup

	listenThread, listenComplete := threads.NewInterruptableThreadComplete("Peer Channel Listen",
		l.listen, &wait)

	handleThread, handleComplete := threads.NewInterruptableThreadComplete("Peer Channel Handle",
		l.handle, &wait)

	listenThread.Start(ctx)
	handleThread.Start(ctx)

	select {
	case <-listenComplete:
	case <-handleComplete:
	case <-interrupt:
	}

	listenThread.Stop(ctx)
	handleThread.Stop(ctx)

	wait.Wait()
	return threads.CombineErrors(listenThread.Error(), handleThread.Error())
}

func (l *PeerChannelsListener[Update]) listen(ctx context.Context,
	interrupt <-chan interface{}) error {
	for {
		logger.Info(ctx, "Connecting to peer channel service to listen for UUID messages")

		if err := l.peerChannelsClient.Listen(ctx, l.readToken, true, l.messagesChannel,
			interrupt); err != nil {
			if errors.Cause(err) == threads.Interrupted {
				return nil
			}

			logger.Warn(ctx, "Peer channel listening returned with error : %s", err)
		} else {
			logger.Warn(ctx, "Peer channel listening returned")
		}

		logger.Warn(ctx, "Waiting to reconnect to Peer channel")
		select {
		case <-time.After(time.Second * 5):
		case <-interrupt:
			return nil
		}
	}
}

func (l *PeerChannelsListener[Update]) handle(ctx context.Context,
	interrupt <-chan interface{}) error {
	for {
		select {
		case msg := <-l.messagesChannel:
			if err := l.handleMessage(ctx, msg); err != nil &&
				errors.Cause(err) != MessageNotRelevent {
				return errors.Wrap(err, "handle message")
			}

			if err := l.peerChannelsClient.MarkMessages(ctx, msg.ChannelID, l.readToken,
				msg.Sequence, true, true); err != nil {
				return errors.Wrap(err, "mark message")
			}

		case update := <-l.updatesChannel:
			if l.handleUpdate == nil {
				return errors.New("Received update with no handler specified")
			}

			if err := l.handleUpdate(ctx, update); err != nil {
				return errors.Wrap(err, "handle update")
			}

		case <-interrupt:
			return nil
		}
	}
}
