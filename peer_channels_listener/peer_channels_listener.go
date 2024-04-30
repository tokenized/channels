package peer_channels_listener

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
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
type HandleUpdate func(ctx context.Context, update interface{}) error

// AddUpdate adds an update struct to be handled in the same thread as the message handler. This
// function interface can be used by the message handler so that there isn't a circular dependency
// between the message handler and the listener.
type AddUpdate func(update interface{}) error

type PeerChannelsListener struct {
	peerChannelsClient peer_channels.Client
	readToken          string
	handleMessage      HandleMessage
	handleUpdate       HandleUpdate
	messagesChannel    chan peer_channels.Message
	updatesChannel     chan interface{}
	handleThreadCount  int
	channelTimeout     atomic.Value
}

func NewPeerChannelsListener(peerChannelsClient peer_channels.Client, readToken string,
	channelSize, handleThreadCount int, channelTimeout time.Duration, handleMessage HandleMessage,
	handleUpdate HandleUpdate) *PeerChannelsListener {
	result := &PeerChannelsListener{
		peerChannelsClient: peerChannelsClient,
		readToken:          readToken,
		handleMessage:      handleMessage,
		handleUpdate:       handleUpdate,
		messagesChannel:    make(chan peer_channels.Message, channelSize),
		updatesChannel:     make(chan interface{}, channelSize),
		handleThreadCount:  handleThreadCount,
	}

	result.channelTimeout.Store(channelTimeout)
	return result
}

func (l *PeerChannelsListener) AddUpdate(update interface{}) error {
	select {
	case l.updatesChannel <- update:
	case <-time.After(l.channelTimeout.Load().(time.Duration)):
		return peer_channels.ErrChannelTimeout
	}

	return nil
}

func (l *PeerChannelsListener) Run(ctx context.Context, interrupt <-chan interface{}) error {
	var listenWait, handleWait sync.WaitGroup
	var selects []reflect.SelectCase

	listenThread, listenComplete := threads.NewInterruptableThreadComplete("Peer Channel Listen",
		func(ctx context.Context, interrupt <-chan interface{}) error {
			return l.listen(ctx, interrupt, CopyString(l.readToken),
				l.channelTimeout.Load().(time.Duration))
		}, &listenWait)
	selects = append(selects, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(listenComplete),
	})

	handleThreads := make([]*threads.InterruptableThread, l.handleThreadCount)
	for i := 0; i < l.handleThreadCount; i++ {
		index := i
		handleThread, handleComplete := threads.NewInterruptableThreadComplete(fmt.Sprintf("Peer Channel Handle %d", index),
			func(ctx context.Context, interrupt <-chan interface{}) error {
				return l.handle(ctx, interrupt, l.handleMessage, l.handleUpdate,
					CopyString(l.readToken))
			}, &handleWait)
		handleThreads[i] = handleThread

		selects = append(selects, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(handleComplete),
		})
	}

	interruptIndex := len(selects)
	selects = append(selects, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(interrupt),
	})

	listenThread.Start(ctx)
	for _, handleThread := range handleThreads {
		handleThread.Start(ctx)
	}

	selectIndex, selectValue, valueReceived := reflect.Select(selects)
	var selectErr error
	if valueReceived {
		selectInterface := selectValue.Interface()
		if selectInterface != nil {
			err, ok := selectInterface.(error)
			if ok {
				selectErr = err
			}
		}
	}

	if selectIndex == 0 {
		logger.Error(ctx, "Peer Channel Listen thread completed : %s", selectErr)
	} else if selectIndex < interruptIndex {
		logger.Error(ctx, "Peer Channel Handle thread %d completed : %s", selectIndex-1, selectErr)
	}

	listenThread.Stop(ctx)
	waitWarning := logger.NewWaitingWarning(ctx, time.Second*3, "Listen thread")
	listenWait.Wait()
	waitWarning.Cancel()

	for _, handleThread := range handleThreads {
		handleThread.Stop(ctx)
	}
	waitWarning = logger.NewWaitingWarning(ctx, time.Second*3, "Handle threads")
	handleWait.Wait()
	waitWarning.Cancel()

	var errs []error
	if err := listenThread.Error(); err != nil {
		errs = append(errs, err)
	}
	for _, handleThread := range handleThreads {
		if err := handleThread.Error(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return threads.CombineErrors(errs...)
	}

	return nil
}

func CopyString(s string) string {
	result := make([]byte, len(s))
	copy(result, s)
	return string(result)
}

func (l *PeerChannelsListener) listen(ctx context.Context, interrupt <-chan interface{},
	readToken string, channelTimeout time.Duration) error {

	for {
		logger.Info(ctx, "Connecting to peer channel service to listen for messages")

		if err := l.peerChannelsClient.Listen(ctx, readToken, true, channelTimeout,
			l.messagesChannel, interrupt); err != nil {
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

func (l *PeerChannelsListener) handle(ctx context.Context, interrupt <-chan interface{},
	handleMessage HandleMessage, handleUpdate HandleUpdate, readToken string) error {
	for {
		select {
		case msg := <-l.messagesChannel:
			if err := handleMessage(ctx, msg); err != nil &&
				errors.Cause(err) != MessageNotRelevent {
				return errors.Wrap(err, "handle message")
			}

			if err := l.peerChannelsClient.MarkMessages(ctx, msg.ChannelID, readToken,
				msg.Sequence, true, true); err != nil {
				return errors.Wrap(err, "mark message")
			}

		case update := <-l.updatesChannel:
			if handleUpdate == nil {
				return errors.New("Received update with no handler specified")
			}

			if err := handleUpdate(ctx, update); err != nil {
				return errors.Wrap(err, "handle update")
			}

		case <-interrupt:
			return nil
		}
	}
}
