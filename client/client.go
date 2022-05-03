package client

import (
	"context"
	"sync"

	"github.com/tokenized/channels"
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
)

type Client struct {
	account       Account
	entity        channels.Entity
	handleMessage HandleMessage

	Channels Channels

	sync.RWMutex
}

type Account struct {
	URL   string
	ID    string
	Token string
}

type HandleMessage func(ctx context.Context, channel *Channel, protocolIDs envelope.ProtocolIDs,
	payload bitcoin.ScriptItems) error

func NewClient(account Account, entity channels.Entity, handleMessage HandleMessage) *Client {
	return &Client{
		account:       account,
		entity:        entity,
		handleMessage: handleMessage,
	}
}

func (c *Client) Listen(ctx context.Context, interrupt <-chan interface{}) error {
	wait := &sync.WaitGroup{}
	incoming := make(chan peer_channels.Message)

	listenThread := threads.NewThread("Peer Channel", func(ctx context.Context,
		interrupt <-chan interface{}) error {
		return peer_channels.AccountListen(ctx, c.account.URL, c.account.ID, c.account.Token,
			incoming, interrupt)
	})
	listenThread.SetWait(wait)
	listenThreadComplete := listenThread.GetCompleteChannel()

	listenThread.Start(ctx)

	select {
	case <-interrupt:
		listenThread.Stop(ctx)

	case <-listenThreadComplete:
		logger.Warn(ctx, "Listen thread stopped : %s", listenThread.Error())
	}

	wait.Wait()

	return listenThread.Error()
}

func (c *Client) AddChannel(ctx context.Context, channel *Channel) error {
	c.Lock()
	defer c.Unlock()

	return nil
}
