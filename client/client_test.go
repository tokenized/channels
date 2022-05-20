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
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/wire"
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
		PublicKey:          user2Channel.Key().PublicKey(),
		PeerChannels:       user2Channel.IncomingPeerChannels(),
		SupportedProtocols: SupportedProtocols(),
		Identity:           *user2Identity,
	}

	if err := user2Channel.SendMessage(ctx, initiation, nil); err != nil {
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
		wMessage, err := channels.Unwrap(channelMessage.Message.Payload())
		if err != nil {
			t.Fatalf("Failed to umwrap message : %s", err)
		}

		if wMessage.Signature == nil {
			t.Errorf("Message not signed")
		}

		if wMessage.Response != nil {
			t.Errorf("Should not be a response")
		}

		if wMessage.Message == nil {
			continue
		}

		js, err := json.MarshalIndent(wMessage.Message, "", "  ")
		t.Logf("User 1 message : %s", js)

		initiation, ok := wMessage.Message.(*channels.RelationshipInitiation)
		if !ok {
			continue
		}
		initiationFound = true

		if !initiation.PublicKey.Equal(user2Channel.Key().PublicKey()) {
			t.Errorf("Wrong public key in initiation : got %s, want %s",
				initiation.PublicKey, user2Channel.Key().PublicKey())
		}

		if initiation.PeerChannels[0].ID != user2Channel.IncomingPeerChannels()[0].ID {
			t.Errorf("Wrong peer channel in initiation : got %s, want %s",
				initiation.PeerChannels[0].ID, user2Channel.IncomingPeerChannels()[0].ID)
		}

		if err := user1Channel.InitializeRelationship(ctx, channelMessage.Message.Payload(),
			initiation); err != nil {
			t.Fatalf("Failed to initialize channel : %s", err)
		}

		// Respond to relationship initiation
		responseInitiation := &channels.RelationshipInitiation{
			PublicKey:          user1Channel.Key().PublicKey(),
			PeerChannels:       user1Channel.IncomingPeerChannels(),
			SupportedProtocols: SupportedProtocols(),
			Identity:           *user1Identity,
		}

		responseID := channelMessage.Message.ID()
		if err := user1Channel.SendMessage(ctx, responseInitiation, &responseID); err != nil {
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
		wMessage, err := channels.Unwrap(channelMessage.Message.Payload())
		if err != nil {
			t.Fatalf("Failed to umwrap message : %s", err)
		}

		if wMessage.Signature == nil {
			t.Errorf("Message not signed")
		}

		if wMessage.Response == nil {
			t.Errorf("Should be a response")
		}

		if wMessage.Message == nil {
			continue
		}

		js, err := json.MarshalIndent(wMessage.Message, "", "  ")
		t.Logf("User 2 message : %s", js)

		msg, ok := wMessage.Message.(*channels.RelationshipInitiation)
		if !ok {
			continue
		}
		responseFound = true

		publicKey := user1Channel.Key().PublicKey()

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

func Test_Response(t *testing.T) {
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

	/************************************** Send Merkle Proof *************************************/
	/**********************************************************************************************/

	paymentKey, err := bitcoin.GenerateKey(bitcoin.MainNet)
	if err != nil {
		t.Fatalf("Failed to generate payment key : %s", err)
	}

	paymentLockingScript, err := paymentKey.LockingScript()
	if err != nil {
		t.Fatalf("Failed to create locking script : %s", err)
	}

	previousTx := wire.NewMsgTx(1)
	previousTx.AddTxOut(wire.NewTxOut(125000, paymentLockingScript))

	fakeMerkleProof := MockMerkleProof(previousTx)

	merkleProof := &channels.MerkleProof{
		MerkleProof: fakeMerkleProof,
	}

	js, _ := json.MarshalIndent(merkleProof, "", "  ")
	t.Logf("Merkle Proof : %s", js)

	if err := user2Channel.SendMessage(ctx, merkleProof, nil); err != nil {
		t.Fatalf("Failed to send Merkle Proof : %s", err)
	}

	time.Sleep(250 * time.Millisecond)

	/************************************ Receive Merkle Proof ************************************/
	/**********************************************************************************************/

	user1Messages, err := client1.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(user1Messages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(user1Messages), 1)
	}

	wMessage, err := channels.Unwrap(user1Messages[0].Message.Payload())
	if err != nil {
		t.Fatalf("Failed to unwrap message : %s", err)
	}

	if _, ok := wMessage.Message.(*channels.MerkleProof); !ok {
		t.Errorf("Message not a merkle proof")
	}

	autoResponse := user1Messages[0].Message.Response()
	if len(autoResponse) == 0 {
		t.Fatalf("No auto response provided")
	}

	wMessage, err = channels.Unwrap(autoResponse)
	if err != nil {
		t.Fatalf("Failed to unwrap message : %s", err)
	}

	reject, ok := wMessage.Message.(*channels.Reject)
	if !ok {
		t.Errorf("Message not a reject")
	}

	js, _ = json.MarshalIndent(reject, "", "  ")
	t.Logf("Reject : %s", js)

	if wMessage.Signature != nil {
		t.Errorf("Auto response should not be signed")
	}

	if wMessage.Response != nil {
		t.Errorf("Auto response should not contain response")
	}

	if wMessage.MessageID != nil {
		t.Errorf("Auto response should not contain message id")
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
