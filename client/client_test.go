package client

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/wallet"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"
)

func Test_Initiate(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	peerChannelsFactory := peer_channels.NewFactory()
	channelClient, _ := peerChannelsFactory.NewClient(peer_channels.MockClientURL)

	/******************************** Create User 1 Client ****************************************/
	/**********************************************************************************************/
	user1Name := "User 1"
	user1Identity := channels.Identity{Name: &user1Name}
	user1AccountID, user1AccountToken, err := channelClient.CreateAccount(ctx, "")
	if err != nil {
		t.Fatalf("Failed to create account : %s", err)
	}

	user1PublicPeerChannel, err := channelClient.CreatePublicChannel(ctx, *user1AccountID,
		*user1AccountToken)
	if err != nil {
		t.Fatalf("Failed to create public channel : %s", err)
	}

	// js, _ := json.MarshalIndent(user1PublicPeerChannel, "", "  ")
	// t.Logf("User 1 Public Peer Channel : %s", js)

	user1Client := NewClient(Account{
		BaseURL: peer_channels.MockClientURL,
		ID:      *user1AccountID,
		Token:   *user1AccountToken,
	}, user1Identity, nil, peerChannelsFactory)

	user1PublicPeerChannels := channels.PeerChannels{
		{
			BaseURL: peer_channels.MockClientURL,
			ID:      user1PublicPeerChannel.ID,
		},
	}
	user1PublicChannel := NewInitiationChannel(user1PublicPeerChannels)
	if err := user1Client.AddChannel(user1PublicChannel); err != nil {
		t.Fatalf("Failed to add channel : %s", err)
	}

	user1PeerChannel, err := channelClient.CreateChannel(ctx, *user1AccountID, *user1AccountToken)
	if err != nil {
		t.Fatalf("Failed to create channel : %s", err)
	}
	user1PeerChannels := channels.PeerChannels{
		{
			BaseURL:    peer_channels.MockClientURL,
			ID:         user1PeerChannel.ID,
			WriteToken: user1PeerChannel.AccessTokens[1].Token,
		},
	}

	user1BaseKey, _ := bitcoin.GenerateKey(bitcoin.MainNet)
	user1ChannelHash, user1ChannelKey := wallet.GenerateHashKey(user1BaseKey, "test")
	user1ChannelPublicKey := user1ChannelKey.PublicKey()

	user1Channel := NewPrivateChannel(user1ChannelHash, user1ChannelPublicKey, user1PeerChannels,
		user1Identity)

	if err := user1Client.AddChannel(user1Channel); err != nil {
		t.Fatalf("Failed to add channel : %s", err)
	}

	/******************************** Create User 2 Client ****************************************/
	/**********************************************************************************************/
	user2Name := "User 2"
	user2Identity := channels.Identity{Name: &user2Name}
	user2AccountID, user2AccountToken, err := channelClient.CreateAccount(ctx, "")
	if err != nil {
		t.Fatalf("Failed to create account : %s", err)
	}

	user2PeerChannel, err := channelClient.CreateChannel(ctx, *user2AccountID, *user2AccountToken)
	if err != nil {
		t.Fatalf("Failed to create channel : %s", err)
	}

	// js, _ = json.MarshalIndent(user2PeerChannel, "", "  ")
	// t.Logf("User 2 Peer Channel : %s", js)

	user2Client := NewClient(Account{
		BaseURL: peer_channels.MockClientURL,
		ID:      *user2AccountID,
		Token:   *user2AccountToken,
	}, user1Identity, nil, peerChannelsFactory)

	user2PeerChannels := channels.PeerChannels{
		{
			BaseURL:    peer_channels.MockClientURL,
			ID:         user2PeerChannel.ID,
			WriteToken: user2PeerChannel.AccessTokens[1].Token,
		},
	}

	user2BaseKey, _ := bitcoin.GenerateKey(bitcoin.MainNet)
	user2ChannelHash, user2ChannelKey := wallet.GenerateHashKey(user2BaseKey, "test")
	user2ChannelPublicKey := user2ChannelKey.PublicKey()

	user2Channel := NewPrivateChannel(user2ChannelHash, user2ChannelPublicKey, user2PeerChannels,
		user2Identity)

	if err := user2Client.AddChannel(user2Channel); err != nil {
		t.Fatalf("Failed to add channel : %s", err)
	}

	/*************************************** Start Clients ****************************************/
	/**********************************************************************************************/
	wait := &sync.WaitGroup{}

	interrupt1 := make(chan interface{})
	wait.Add(1)
	go func() {
		user1Client.Run(ctx, interrupt1)
		wait.Done()
	}()

	interrupt2 := make(chan interface{})
	wait.Add(1)
	go func() {
		user2Client.Run(ctx, interrupt1)
		wait.Done()
	}()

	/********************************** Send Initiation Message ***********************************/
	/**********************************************************************************************/
	initiation := &channels.RelationshipInitiation{
		PublicKey:          user2ChannelPublicKey,
		PeerChannels:       user2PeerChannels,
		SupportedProtocols: SupportedProtocols(),
		Identity:           user2Identity,
	}

	initProtocolIDs, initPayload, err := channels.WriteRelationship(initiation)
	if err != nil {
		t.Fatalf("Failed to write initiation message : %s", err)
	}

	initHash := randHash()
	initProtocolIDs, initPayload, err = channels.Sign(initProtocolIDs, initPayload, user2ChannelKey,
		&initHash, false)
	if err != nil {
		t.Fatalf("Failed to sign initiation message : %s", err)
	}

	hash, err := SendMessage(ctx, peerChannelsFactory, user1PublicPeerChannels, initProtocolIDs,
		initPayload)
	if err != nil {
		t.Fatalf("Failed to send initiation : %s", err)
	}

	user2Channel.Outgoing.AddMessage(ctx, *hash, initProtocolIDs, initPayload)

	/******************************** Respond to Initiation Message *******************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	user1Messages, err := user1Client.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(user1Messages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(user1Messages), 1)
	}

	initiationFound := false
	for _, channelMessage := range user1Messages {
		message := channelMessage.Message

		_, protocolIDs, payload, err := channels.ParseSigned(message.ProtocolIDs, message.Payload)
		if err != nil {
			t.Fatalf("Failed to parse signature : %s", err)
		}

		relationshipMsg, err := channels.ParseRelationship(protocolIDs, payload)
		if err != nil {
			t.Fatalf("Failed to parse relationship : %s", err)
		}

		if relationshipMsg == nil {
			continue
		}

		js, err := json.MarshalIndent(relationshipMsg, "", "  ")
		t.Logf("User 1 message : %s", js)

		initiation, ok := relationshipMsg.(*channels.RelationshipInitiation)
		if !ok {
			continue
		}
		initiationFound = true

		if !initiation.PublicKey.Equal(user2ChannelPublicKey) {
			t.Errorf("Wrong public key in initiation : got %s, want %s",
				initiation.PublicKey, user2ChannelPublicKey)
		}

		if initiation.PeerChannels[0].ID != user2PeerChannel.ID {
			t.Errorf("Wrong peer channel in initiation : got %s, want %s",
				initiation.PeerChannels[0].ID, user2PeerChannel.ID)
		}

		// Respond to relationship initiation
		responseInitiation := &channels.RelationshipInitiation{
			PublicKey:          user1ChannelPublicKey,
			PeerChannels:       user1PeerChannels,
			SupportedProtocols: SupportedProtocols(),
			Identity:           user1Identity,
		}

		responseProtocolIDs, responsePayload, err := channels.WriteRelationship(responseInitiation)
		if err != nil {
			t.Fatalf("Failed to write initiation message : %s", err)
		}

		responseHash := randHash()
		responseProtocolIDs, responsePayload, err = channels.Sign(responseProtocolIDs,
			responsePayload, user1ChannelKey, &responseHash, false)
		if err != nil {
			t.Fatalf("Failed to sign initiation message : %s", err)
		}

		hash, err := SendMessage(ctx, peerChannelsFactory, initiation.PeerChannels,
			responseProtocolIDs, responsePayload)
		if err != nil {
			t.Fatalf("Failed to send initiation : %s", err)
		}

		user1Channel.Outgoing.AddMessage(ctx, *hash, responseProtocolIDs, responsePayload)
	}

	if !initiationFound {
		t.Errorf("Initiation not found")
	}

	/***************************** Receive Initiation Response Message ****************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	user2Messages, err := user2Client.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(user2Messages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(user2Messages), 1)
	}

	responseFound := false
	for _, channelMessage := range user2Messages {
		message := channelMessage.Message

		_, protocolIDs, payload, err := channels.ParseSigned(message.ProtocolIDs, message.Payload)
		if err != nil {
			t.Fatalf("Failed to parse signature : %s", err)
		}

		relationshipMsg, err := channels.ParseRelationship(protocolIDs, payload)
		if err != nil {
			t.Fatalf("Failed to parse relationship : %s", err)
		}

		if relationshipMsg == nil {
			continue
		}

		js, err := json.MarshalIndent(relationshipMsg, "", "  ")
		t.Logf("User 2 message : %s", js)

		msg, ok := relationshipMsg.(*channels.RelationshipInitiation)
		if !ok {
			continue
		}
		responseFound = true

		if !msg.PublicKey.Equal(user1ChannelPublicKey) {
			t.Errorf("Wrong public key in initiation response : got %s, want %s",
				msg.PublicKey, user1ChannelPublicKey)
		}

		if msg.PeerChannels[0].ID != user1PeerChannel.ID {
			t.Errorf("Wrong peer channel in initiation response : got %s, want %s",
				msg.PeerChannels[0].ID, user1PeerChannel.ID)
		}
	}

	if !responseFound {
		t.Errorf("Initiation response not found")
	}

	close(interrupt1)
	close(interrupt2)
	wait.Wait()
}

func randHash() bitcoin.Hash32 {
	var hash bitcoin.Hash32
	rand.Read(hash[:])
	return hash
}
