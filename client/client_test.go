package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/tokenized/channels"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"
)

func Test_Initiate(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	peerChannelsFactory := peer_channels.NewFactory()
	peerClient, _ := peerChannelsFactory.NewClient(peer_channels.MockClientURL)

	/******************************** Create User 1 Client ****************************************/
	/**********************************************************************************************/
	user1 := CreateMockUser(ctx, peerChannelsFactory, "User 1")
	user1PublicChannel := user1.CreateInitiationChannel(ctx, peerClient)
	user1Channel := user1.CreateChannel(ctx, peerClient)

	if user1Channel.Outgoing.Entity != nil {
		t.Errorf("User 1 outgoing entity should be nil")
	}

	/******************************** Create User 2 Client ****************************************/
	/**********************************************************************************************/
	user2 := CreateMockUser(ctx, peerChannelsFactory, "User 2")
	user2Channel := user2.CreateChannel(ctx, peerClient)

	if user2Channel.Outgoing.Entity != nil {
		t.Errorf("User 2 outgoing entity should be nil")
	}

	/*************************************** Start Clients ****************************************/
	/**********************************************************************************************/
	wait := &sync.WaitGroup{}

	interrupt1 := make(chan interface{})
	wait.Add(1)
	go func() {
		user1.Client.Run(ctx, interrupt1)
		wait.Done()
	}()

	interrupt2 := make(chan interface{})
	wait.Add(1)
	go func() {
		user2.Client.Run(ctx, interrupt1)
		wait.Done()
	}()

	/********************************** Send Initiation Message ***********************************/
	/**********************************************************************************************/
	initiation := &channels.RelationshipInitiation{
		PublicKey:          user2.HashKey(user2Channel.Hash()).PublicKey(),
		PeerChannels:       user2Channel.Incoming.Entity.PeerChannels,
		SupportedProtocols: SupportedProtocols(),
		Identity:           user2.Client.Identity,
	}

	initPayload, err := initiation.Write()
	if err != nil {
		t.Fatalf("Failed to write initiation message : %s", err)
	}

	initRandHash := randHash()
	signature, err := channels.Sign(initPayload, user2.HashKey(user2Channel.Hash()), &initRandHash,
		false)
	if err != nil {
		t.Fatalf("Failed to sign initiation message : %s", err)
	}

	initPayload, err = signature.Wrap(initPayload)
	if err != nil {
		t.Fatalf("Failed to wrap initiation message : %s", err)
	}

	scriptItems := envelopeV1.Wrap(initPayload)
	script, err := scriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create script : %s", err)
	}

	initHash := bitcoin.Hash32(sha256.Sum256(script))

	if err := SendMessage(ctx, peerChannelsFactory, user1PublicChannel.Incoming.Entity.PeerChannels,
		script); err != nil {
		t.Fatalf("Failed to send initiation : %s", err)
	}

	user2Channel.Outgoing.AddMessage(ctx, script)

	/******************************** Respond to Initiation Message *******************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	user1Messages, err := user1.Client.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(user1Messages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(user1Messages), 1)
	}

	initiationFound := false
	for _, channelMessage := range user1Messages {
		message := channelMessage.Message

		payload, err := envelopeV1.Parse(bytes.NewReader(message.Payload))
		if err != nil {
			t.Fatalf("Failed to parse envelope : %s", err)
		}

		_, payload, err = channels.ParseSigned(payload)
		if err != nil {
			t.Fatalf("Failed to parse signature : %s", err)
		}

		var receivedResponse *channels.Response
		receivedResponse, payload, err = channels.ParseResponse(payload)
		if err != nil {
			t.Fatalf("Failed to parse channels : %s", err)
		}
		if receivedResponse != nil {
			t.Errorf("Should not be a response")
		}

		relationshipMsg, err := channels.ParseRelationship(payload)
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

		if !initiation.PublicKey.Equal(user2.HashKey(user2Channel.Hash()).PublicKey()) {
			t.Errorf("Wrong public key in initiation : got %s, want %s",
				initiation.PublicKey, user2.HashKey(user2Channel.Hash()).PublicKey())
		}

		if initiation.PeerChannels[0].ID != user2Channel.Incoming.Entity.PeerChannels[0].ID {
			t.Errorf("Wrong peer channel in initiation : got %s, want %s",
				initiation.PeerChannels[0].ID, user2Channel.Incoming.Entity.PeerChannels[0].ID)
		}

		// Respond to relationship initiation
		responseInitiation := &channels.RelationshipInitiation{
			PublicKey:          user1Channel.Incoming.Entity.PublicKey,
			PeerChannels:       user1Channel.Incoming.Entity.PeerChannels,
			SupportedProtocols: SupportedProtocols(),
			Identity:           user1.Client.Identity,
		}

		responsePayload, err := responseInitiation.Write()
		if err != nil {
			t.Fatalf("Failed to write initiation message : %s", err)
		}

		response := &channels.Response{
			MessageHash: bitcoin.Hash32(sha256.Sum256(message.Payload)),
		}

		if !response.MessageHash.Equal(&initHash) {
			t.Errorf("Wrong message hash for response : got %s, want %s", response.MessageHash,
				initHash)
		}

		responsePayload, err = response.Wrap(responsePayload)
		if err != nil {
			t.Fatalf("Failed to write response wrapper : %s", err)
		}

		responseHash := randHash()
		signature, err = channels.Sign(responsePayload, user1.HashKey(user1Channel.Hash()),
			&responseHash, false)
		if err != nil {
			t.Fatalf("Failed to sign initiation message : %s", err)
		}

		responsePayload, err = signature.Wrap(responsePayload)
		if err != nil {
			t.Fatalf("Failed to wrap initiation message : %s", err)
		}

		scriptItems := envelopeV1.Wrap(responsePayload)
		script, err := scriptItems.Script()
		if err != nil {
			t.Fatalf("Failed to create script : %s", err)
		}

		if err := SendMessage(ctx, peerChannelsFactory, initiation.PeerChannels,
			script); err != nil {
			t.Fatalf("Failed to send initiation : %s", err)
		}

		user1Channel.Outgoing.AddMessage(ctx, script)
	}

	if !initiationFound {
		t.Errorf("Initiation not found")
	}

	if user2Channel.Outgoing.Entity != nil {
		t.Errorf("User 2 outgoing entity should be nil")
	}

	/***************************** Receive Initiation Response Message ****************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	user2Messages, err := user2.Client.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(user2Messages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(user2Messages), 1)
	}

	responseFound := false
	for _, channelMessage := range user2Messages {
		message := channelMessage.Message

		payload, err := envelopeV1.Parse(bytes.NewReader(message.Payload))
		if err != nil {
			t.Fatalf("Failed to parse envelope : %s", err)
		}

		_, payload, err = channels.ParseSigned(payload)
		if err != nil {
			t.Fatalf("Failed to parse signature : %s", err)
		}

		var receivedResponse *channels.Response
		receivedResponse, payload, err = channels.ParseResponse(payload)
		if err != nil {
			t.Fatalf("Failed to parse response : %s", err)
		}
		if receivedResponse == nil {
			t.Errorf("Response does not contain response header")
		}

		relationshipMsg, err := channels.ParseRelationship(payload)
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

		publicKey := user1.HashKey(user1Channel.Hash()).PublicKey()

		if !msg.PublicKey.Equal(publicKey) {
			t.Errorf("Wrong public key in initiation response : got %s, want %s",
				msg.PublicKey, publicKey)
		}

		if msg.PeerChannels[0].ID != user1Channel.Incoming.Entity.PeerChannels[0].ID {
			t.Errorf("Wrong peer channel in initiation response : got %s, want %s",
				msg.PeerChannels[0].ID, user1Channel.Incoming.Entity.PeerChannels[0].ID)
		}
	}

	if !responseFound {
		t.Errorf("Initiation response not found")
	}

	if user2Channel.Outgoing.Entity == nil {
		t.Errorf("User 2 outgoing entity should not be nil")
	}

	/**************************************** Stop Clients ****************************************/
	/**********************************************************************************************/

	close(interrupt1)
	close(interrupt2)
	wait.Wait()
}

func randHash() bitcoin.Hash32 {
	var hash bitcoin.Hash32
	rand.Read(hash[:])
	return hash
}
