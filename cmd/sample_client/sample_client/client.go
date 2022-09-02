package sample_client

import (
	"context"
	"fmt"
	"sync"

	"github.com/tokenized/channels"
	channelsClient "github.com/tokenized/channels/client"
	"github.com/tokenized/channels/wallet"
	"github.com/tokenized/logger"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"
	spyNodeClient "github.com/tokenized/spynode/pkg/client"
	"github.com/tokenized/threads"

	"github.com/pkg/errors"
)

type Client struct {
	ChannelsClient *channelsClient.Client
	Wallet         *wallet.Wallet
	store          storage.StreamStorage

	spyNodeClient spyNodeClient.Client
}

var (
	clientHash, _ = bitcoin.NewHash32FromStr("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	walletHash, _ = bitcoin.NewHash32FromStr("fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321")
)

func NewClient(key bitcoin.Key, store storage.StreamStorage, protocols *channels.Protocols,
	peerChannelsFactory *peer_channels.Factory, merkleProofVerifier wallet.MerkleProofVerifier,
	feeQuoter wallet.FeeQuoter, spyNodeClient spyNodeClient.Client) *Client {

	clientKey, err := key.AddHash(*clientHash)
	if err != nil {
		panic(fmt.Sprintf("Failed to generate client key : %s", err))
	}

	walletKey, err := key.AddHash(*walletHash)
	if err != nil {
		panic(fmt.Sprintf("Failed to generate wallet key : %s", err))
	}

	wallet := wallet.NewWallet(wallet.DefaultConfig(), store, merkleProofVerifier, feeQuoter,
		walletKey)

	return &Client{
		ChannelsClient: channelsClient.NewClient(clientKey, store, protocols, wallet,
			peerChannelsFactory),
		Wallet:        wallet,
		spyNodeClient: spyNodeClient,
		store:         store,
	}
}

func (c *Client) Load(ctx context.Context) error {
	if err := c.ChannelsClient.Load(ctx); err != nil {
		return errors.Wrap(err, "channels client")
	}

	if err := c.Wallet.Load(ctx); err != nil {
		return errors.Wrap(err, "wallet")
	}

	return nil
}

func (c *Client) Save(ctx context.Context) error {
	if err := c.ChannelsClient.Save(ctx); err != nil {
		return errors.Wrap(err, "channels client")
	}

	if err := c.Wallet.Save(ctx); err != nil {
		return errors.Wrap(err, "wallet")
	}

	return nil
}

func (c *Client) Run(ctx context.Context, interrupt <-chan interface{}) error {
	var wait sync.WaitGroup

	clientThread, clientComplete := threads.NewInterruptableThreadComplete("Client",
		c.ChannelsClient.Run, &wait)

	incomingMessages := c.ChannelsClient.GetIncomingChannel(ctx)
	incomingThread, incomingComplete := threads.NewUninterruptableThreadComplete("Incoming",
		func(ctx context.Context) error {
			for msg := range incomingMessages {
				if err := c.handleMessage(ctx, msg); err != nil {
					return err
				}
			}

			return nil
		}, &wait)

	clientThread.Start(ctx)
	incomingThread.Start(ctx)

	select {
	case <-interrupt:

	case <-clientComplete:
		logger.Error(ctx, "Client thread completed : %s", clientThread.Error())

	case <-incomingComplete:
		logger.Error(ctx, "Incoming thread completed : %s", incomingThread.Error())
	}

	clientThread.Stop(ctx)
	c.ChannelsClient.CloseIncomingChannel(ctx)

	wait.Wait()
	return nil
}

func (c *Client) handleMessage(ctx context.Context, msg channelsClient.ChannelMessage) error {
	return nil
}
