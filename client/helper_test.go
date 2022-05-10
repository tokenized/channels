package client

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/wire"
)

type MockUser struct {
	BaseKey bitcoin.Key
	Client  *Client
}

func MockRelatedUsers(ctx context.Context,
	peerChannelsFactory *peer_channels.Factory) (*Client, *Client) {

	userAName := "User A"
	userAIdentity := &channels.Identity{
		Name: &userAName,
	}
	clientA := MockClient(ctx, peerChannelsFactory)

	userBName := "User B"
	userBIdentity := &channels.Identity{
		Name: &userBName,
	}
	clientB := MockClient(ctx, peerChannelsFactory)

	channelA, err := clientA.CreatePrivateChannel(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to create channel : %s", err))
	}

	channelB, err := clientB.CreatePrivateChannel(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to create channel : %s", err))
	}

	userBInitiation := &channels.RelationshipInitiation{
		PublicKey:          clientB.ChannelKey(channelB).PublicKey(),
		PeerChannels:       channelB.IncomingPeerChannels(),
		SupportedProtocols: SupportedProtocols(),
		Identity:           *userBIdentity,
	}

	if err := channelA.Initialize(ctx, userBInitiation); err != nil {
		panic(fmt.Sprintf("Failed to initialize channel : %s", err))
	}

	userAInitiation := &channels.RelationshipInitiation{
		PublicKey:          clientA.ChannelKey(channelA).PublicKey(),
		PeerChannels:       channelA.IncomingPeerChannels(),
		SupportedProtocols: SupportedProtocols(),
		Identity:           *userAIdentity,
	}

	if err := channelB.Initialize(ctx, userAInitiation); err != nil {
		panic(fmt.Sprintf("Failed to initialize channel : %s", err))
	}

	return clientA, clientB
}

func MockClient(ctx context.Context, peerChannelsFactory *peer_channels.Factory) *Client {
	peerClient, _ := peerChannelsFactory.NewClient(peer_channels.MockClientURL)

	accountID, accountToken, err := peerClient.CreateAccount(ctx, "")
	if err != nil {
		panic(fmt.Sprintf("Failed to create account : %s", err))
	}

	key, err := bitcoin.GenerateKey(bitcoin.MainNet)
	if err != nil {
		panic(fmt.Sprintf("Failed to create key : %s", err))
	}

	return NewClient(key,
		Account{
			BaseURL: peer_channels.MockClientURL,
			ID:      *accountID,
			Token:   *accountToken,
		}, peerChannelsFactory)
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
