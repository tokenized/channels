package client

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/wallet"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/wire"
)

type MockUser struct {
	BaseKey bitcoin.Key
	Client  *Client
}

func MockRelatedUsers(ctx context.Context, store storage.StreamReadWriter,
	peerChannelsFactory *peer_channels.Factory) (*Client, *Client) {

	userAName := "User A"
	userAIdentity := &channels.Identity{
		Name: &userAName,
	}
	clientA := MockClient(ctx, store, peerChannelsFactory)

	userBName := "User B"
	userBIdentity := &channels.Identity{
		Name: &userBName,
	}
	clientB := MockClient(ctx, store, peerChannelsFactory)

	channelA, err := clientA.CreateRelationshipChannel(ctx, wallet.RandomHash())
	if err != nil {
		panic(fmt.Sprintf("Failed to create channel : %s", err))
	}

	channelB, err := clientB.CreateRelationshipChannel(ctx, wallet.RandomHash())
	if err != nil {
		panic(fmt.Sprintf("Failed to create channel : %s", err))
	}

	userBInitiation := &channels.RelationshipInitiation{
		Configuration: channels.ChannelConfiguration{
			PublicKey:          channelB.Key().PublicKey(),
			PeerChannels:       channelB.IncomingPeerChannels(),
			SupportedProtocols: SupportedProtocols(),
		},
		Identity: *userBIdentity,
	}

	initBMessage, err := channelB.CreateMessage(ctx, userBInitiation, nil)
	if err != nil {
		panic(fmt.Sprintf("Failed to create message : %s", err))
	}

	if err := channelA.InitializeRelationship(ctx, initBMessage.Payload(),
		userBInitiation.Configuration.PublicKey,
		userBInitiation.Configuration.PeerChannels); err != nil {
		panic(fmt.Sprintf("Failed to initialize channel : %s", err))
	}

	userAInitiation := &channels.RelationshipInitiation{
		Configuration: channels.ChannelConfiguration{
			PublicKey:          channelA.Key().PublicKey(),
			PeerChannels:       channelA.IncomingPeerChannels(),
			SupportedProtocols: SupportedProtocols(),
		},
		Identity: *userAIdentity,
	}

	initAMessage, err := channelA.CreateMessage(ctx, userAInitiation, nil)
	if err != nil {
		panic(fmt.Sprintf("Failed to create message : %s", err))
	}

	if err := channelB.InitializeRelationship(ctx, initAMessage.Payload(),
		userAInitiation.Configuration.PublicKey,
		userAInitiation.Configuration.PeerChannels); err != nil {
		panic(fmt.Sprintf("Failed to initialize channel : %s", err))
	}

	return clientA, clientB
}

func MockClient(ctx context.Context, store storage.StreamReadWriter,
	peerChannelsFactory *peer_channels.Factory) *Client {

	peerClient, err := peerChannelsFactory.NewClient(peer_channels.MockClientURL)
	if err != nil {
		panic(fmt.Sprintf("Failed to create peer client : %s", err))
	}

	accountID, accountToken, err := peerClient.CreateAccount(ctx, "")
	if err != nil {
		panic(fmt.Sprintf("Failed to create account : %s", err))
	}

	key, err := bitcoin.GenerateKey(bitcoin.MainNet)
	if err != nil {
		panic(fmt.Sprintf("Failed to create key : %s", err))
	}

	client := NewClient(key, store, peerChannelsFactory)
	client.SetPeerChannelsURL(peer_channels.MockClientURL)
	client.SetPeerChannelsAccount(*accountID, *accountToken)
	return client
}

func MockMerkleProof(tx *wire.MsgTx) *merkle_proof.MerkleProof {
	tree := merkle_proof.NewMerkleTree(true)
	txid := *tx.TxHash()
	tree.AddMerkleProof(txid)

	txCount := rand.Intn(1000)
	offset := rand.Intn(txCount)
	for i := 0; i < txCount; i++ {
		if i == offset {
			tree.AddHash(txid)
			continue
		}

		var otherTxid bitcoin.Hash32
		rand.Read(otherTxid[:])
		tree.AddHash(otherTxid)
	}

	_, proofs := tree.FinalizeMerkleProofs()
	proofs[0].Tx = tx
	proofs[0].BlockHash = &bitcoin.Hash32{}
	rand.Read(proofs[0].BlockHash[:])
	return proofs[0]
}
