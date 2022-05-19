package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/wallet"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"
)

func Test_Initiate(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	store := storage.NewMockStorage()
	peerChannelsFactory := peer_channels.NewFactory()

	/******************************** Create User 1 Client ****************************************/
	/**********************************************************************************************/
	client1 := MockClient(ctx, store, peerChannelsFactory)
	user1PublicChannel, err := client1.CreateRelationshipInitiationChannel(ctx, wallet.RandomHash())
	if err != nil {
		t.Fatalf("Failed to create channel : %s", err)
	}
	user1Channel, err := client1.CreateRelationshipChannel(ctx, wallet.RandomHash())
	if err != nil {
		t.Fatalf("Failed to create channel : %s", err)
	}
	user1Name := "User 1"
	user1Identity := &channels.Identity{
		Name: &user1Name,
	}

	/******************************** Create User 2 Client ****************************************/
	/**********************************************************************************************/
	client2 := MockClient(ctx, store, peerChannelsFactory)
	user2Channel, err := client2.CreateRelationshipChannel(ctx, wallet.RandomHash())
	if err != nil {
		t.Fatalf("Failed to create channel : %s", err)
	}

	if err := user2Channel.SetOutgoingPeerChannels(user1PublicChannel.IncomingPeerChannels()); err != nil {
		t.Fatalf("Failed to set peer channels : %s", err)
	}

	user2Name := "User 2"
	user2Identity := &channels.Identity{
		Name: &user2Name,
	}

	/*************************************** Start Clients ****************************************/
	/**********************************************************************************************/
	wait := &sync.WaitGroup{}

	interrupt1 := make(chan interface{})
	wait.Add(1)
	go func() {
		client1.Run(ctx, interrupt1)
		wait.Done()
	}()

	interrupt2 := make(chan interface{})
	wait.Add(1)
	go func() {
		client2.Run(ctx, interrupt1)
		wait.Done()
	}()

	incoming1 := client1.GetIncomingChannel(ctx)
	incoming1Count := 0
	wait.Add(1)
	go func() {
		for range incoming1 {
			t.Logf("Received incoming message 1")
			incoming1Count++
		}
		wait.Done()
	}()

	incoming2 := client2.GetIncomingChannel(ctx)
	incoming2Count := 0
	wait.Add(1)
	go func() {
		for range incoming2 {
			t.Logf("Received incoming message 2")
			incoming2Count++
		}
		wait.Done()
	}()

	/********************************** Send Initiation Message ***********************************/
	/**********************************************************************************************/
	initiation := &channels.RelationshipInitiation{
		PublicKey:          client2.ChannelKey(user2Channel).PublicKey(),
		PeerChannels:       user2Channel.IncomingPeerChannels(),
		SupportedProtocols: SupportedProtocols(),
		Identity:           *user2Identity,
	}

	initPayload, err := initiation.Write()
	if err != nil {
		t.Fatalf("Failed to write initiation message : %s", err)
	}

	initRandHash := randHash()
	signature, err := channels.Sign(initPayload, client2.ChannelKey(user2Channel), &initRandHash,
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

	initiationMessage, err := user2Channel.NewMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to create a new message : %s", err)
	}
	initiationMessage.SetPayload(script)
	if err := user2Channel.SendMessage(ctx, initiationMessage); err != nil {
		t.Fatalf("Failed to send initiation : %s", err)
	}

	/******************************** Respond to Initiation Message *******************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	user1Messages, err := client1.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(user1Messages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(user1Messages), 1)
	}

	initiationFound := false
	for _, channelMessage := range user1Messages {
		message := channelMessage.Message

		payload, err := envelopeV1.Parse(bytes.NewReader(message.Payload()))
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

		if !initiation.PublicKey.Equal(client2.ChannelKey(user2Channel).PublicKey()) {
			t.Errorf("Wrong public key in initiation : got %s, want %s",
				initiation.PublicKey, client2.ChannelKey(user2Channel).PublicKey())
		}

		if initiation.PeerChannels[0].ID != user2Channel.IncomingPeerChannels()[0].ID {
			t.Errorf("Wrong peer channel in initiation : got %s, want %s",
				initiation.PeerChannels[0].ID, user2Channel.IncomingPeerChannels()[0].ID)
		}

		if err := user1Channel.InitializeRelationship(ctx, initiation); err != nil {
			t.Fatalf("Failed to initialize channel : %s", err)
		}

		// Respond to relationship initiation
		responseInitiation := &channels.RelationshipInitiation{
			PublicKey:          client1.ChannelKey(user1Channel).PublicKey(),
			PeerChannels:       user1Channel.IncomingPeerChannels(),
			SupportedProtocols: SupportedProtocols(),
			Identity:           *user1Identity,
		}

		responsePayload, err := responseInitiation.Write()
		if err != nil {
			t.Fatalf("Failed to write initiation message : %s", err)
		}

		response := &channels.Response{
			MessageID: message.ID(),
		}

		if response.MessageID != 0 {
			t.Errorf("Wrong message id for response : got %d, want %d", response.MessageID, 0)
		}

		responsePayload, err = response.Wrap(responsePayload)
		if err != nil {
			t.Fatalf("Failed to write response wrapper : %s", err)
		}

		responseHash := randHash()
		signature, err = channels.Sign(responsePayload, client1.ChannelKey(user1Channel),
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

		responseMessage, err := user1Channel.NewMessage(ctx)
		if err != nil {
			t.Fatalf("Failed to create a new message : %s", err)
		}
		responseMessage.SetPayload(script)
		if err := user1Channel.SendMessage(ctx, responseMessage); err != nil {
			t.Fatalf("Failed to send initiation : %s", err)
		}
	}

	if !initiationFound {
		t.Errorf("Initiation not found")
	}

	/***************************** Receive Initiation Response Message ****************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	user2Messages, err := client2.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(user2Messages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(user2Messages), 1)
	}

	responseFound := false
	for _, channelMessage := range user2Messages {
		message := channelMessage.Message

		payload, err := envelopeV1.Parse(bytes.NewReader(message.Payload()))
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

		publicKey := client1.ChannelKey(user1Channel).PublicKey()

		if !msg.PublicKey.Equal(publicKey) {
			t.Errorf("Wrong public key in initiation response : got %s, want %s",
				msg.PublicKey, publicKey)
		}

		if msg.PeerChannels[0].ID != user1Channel.IncomingPeerChannels()[0].ID {
			t.Errorf("Wrong peer channel in initiation response : got %s, want %s",
				msg.PeerChannels[0].ID, user1Channel.IncomingPeerChannels()[0].ID)
		}
	}

	if !responseFound {
		t.Errorf("Initiation response not found")
	}

	/**************************************** Stop Clients ****************************************/
	/**********************************************************************************************/

	if incoming1Count != 1 {
		t.Errorf("Wrong incoming 1 count : got %d, want %d", incoming1Count, 1)
	}

	if incoming2Count != 1 {
		t.Errorf("Wrong incoming 2 count : got %d, want %d", incoming2Count, 1)
	}

	client1.CloseIncomingChannel(ctx)
	client2.CloseIncomingChannel(ctx)
	close(interrupt1)
	close(interrupt2)
	wait.Wait()
}

func randHash() bitcoin.Hash32 {
	var hash bitcoin.Hash32
	rand.Read(hash[:])
	return hash
}
