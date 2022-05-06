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
)

type MockUser struct {
	BaseKey bitcoin.Key
	Client  *Client
}

func MockRelatedUsers(ctx context.Context,
	peerChannelsFactory *peer_channels.Factory) (*MockUser, *MockUser) {

	peerClient, _ := peerChannelsFactory.NewClient(peer_channels.MockClientURL)

	userA := CreateMockUser(ctx, peerChannelsFactory, "User A")
	userB := CreateMockUser(ctx, peerChannelsFactory, "User B")

	channelA := userA.CreateChannel(ctx, peerClient)
	channelB := userB.CreateChannel(ctx, peerClient)

	channelA.Outgoing.SetEntity(channelB.Incoming.Entity)
	channelB.Outgoing.SetEntity(channelA.Incoming.Entity)

	return userA, userB
}

func CreateMockUser(ctx context.Context, peerChannelsFactory *peer_channels.Factory,
	userName string) *MockUser {

	peerClient, _ := peerChannelsFactory.NewClient(peer_channels.MockClientURL)

	identity := channels.Identity{Name: &userName}
	accountID, accountToken, err := peerClient.CreateAccount(ctx, "")
	if err != nil {
		panic(fmt.Sprintf("Failed to create account : %s", err))
	}

	client := NewClient(Account{
		BaseURL: peer_channels.MockClientURL,
		ID:      *accountID,
		Token:   *accountToken,
	}, identity, peerChannelsFactory)

	key, err := bitcoin.GenerateKey(bitcoin.MainNet)
	if err != nil {
		panic(fmt.Sprintf("Failed to create key : %s", err))
	}

	return &MockUser{
		BaseKey: key,
		Client:  client,
	}
}

func getWriteToken(c *peer_channels.Channel) string {
	for _, token := range c.AccessTokens {
		if token.CanWrite {
			return token.Token
		}
	}

	return ""
}

func (u *MockUser) CreateChannel(ctx context.Context,
	peerClient peer_channels.Client) *Channel {

	peerChannel, err := peerClient.CreateChannel(ctx, u.Client.Account.ID, u.Client.Account.Token)
	if err != nil {
		panic(fmt.Sprintf("Failed to create channel : %s", err))
	}

	peerChannels := channels.PeerChannels{
		{
			BaseURL:    peer_channels.MockClientURL,
			ID:         peerChannel.ID,
			WriteToken: getWriteToken(peerChannel),
		},
	}

	hash, key := wallet.GenerateHashKey(u.BaseKey, "test")
	publicKey := key.PublicKey()

	channel := NewPrivateChannel(hash, publicKey, peerChannels, u.Client.Identity)

	if err := u.Client.AddChannel(channel); err != nil {
		panic(fmt.Sprintf("Failed to add channel : %s", err))
	}

	return channel
}

func (u *MockUser) CreateInitiationChannel(ctx context.Context,
	peerClient peer_channels.Client) *Channel {

	peerChannel, err := peerClient.CreatePublicChannel(ctx, u.Client.Account.ID,
		u.Client.Account.Token)
	if err != nil {
		panic(fmt.Sprintf("Failed to create channel : %s", err))
	}

	peerChannels := channels.PeerChannels{
		{
			BaseURL: peer_channels.MockClientURL,
			ID:      peerChannel.ID,
		},
	}

	channel := NewInitiationChannel(peerChannels)
	if err := u.Client.AddChannel(channel); err != nil {
		panic(fmt.Sprintf("Failed to add channel : %s", err))
	}

	return channel
}

func (u *MockUser) HashKey(hash bitcoin.Hash32) bitcoin.Key {
	key, err := u.BaseKey.AddHash(hash)
	if err != nil {
		panic(fmt.Sprintf("Failed to add hash to key : %s", err))
	}

	return key
}

func MockMerkleProof(txid bitcoin.Hash32) *merkle_proof.MerkleProof {
	tree := merkle_proof.NewMerkleTree(true)
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
	proofs[0].BlockHash = &bitcoin.Hash32{}
	rand.Read(proofs[0].BlockHash[:])
	return proofs[0]
}
