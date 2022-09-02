package client

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/merkle_proofs"
	"github.com/tokenized/channels/relationships"
	"github.com/tokenized/channels/wallet"
	"github.com/tokenized/logger"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/wire"
)

func Test_Initiate(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	store := storage.NewMockStorage()
	peerChannelsFactory := peer_channels.NewFactory()
	protocols := BuildChannelsProtocols()

	/******************************** Create User 1 Client ****************************************/
	/**********************************************************************************************/
	client1 := MockClient(ctx, store, protocols, peerChannelsFactory)
	user1PublicChannel, err := client1.CreateRelationshipInitiationChannel(ctx, wallet.RandomHash())
	if err != nil {
		t.Fatalf("Failed to create channel : %s", err)
	}
	user1Channel, err := client1.CreateRelationshipChannel(ctx, wallet.RandomHash())
	if err != nil {
		t.Fatalf("Failed to create channel : %s", err)
	}
	user1Name := "User 1"
	user1Identity := &relationships.Identity{
		Name: &user1Name,
	}

	/******************************** Create User 2 Client ****************************************/
	/**********************************************************************************************/
	client2 := MockClient(ctx, store, protocols, peerChannelsFactory)
	user2Channel, err := client2.CreateRelationshipChannel(ctx, wallet.RandomHash())
	if err != nil {
		t.Fatalf("Failed to create channel : %s", err)
	}

	if err := user2Channel.SetOutgoingPeerChannels(user1PublicChannel.IncomingPeerChannels()); err != nil {
		t.Fatalf("Failed to set peer channels : %s", err)
	}

	user2Name := "User 2"
	user2Identity := &relationships.Identity{
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

	time.Sleep(time.Millisecond * 250)

	/********************************** Send Initiation Message ***********************************/
	/**********************************************************************************************/
	initiation := &relationships.Initiation{
		Configuration: relationships.ChannelConfiguration{
			PublicKey:          user2Channel.Key().PublicKey(),
			PeerChannels:       user2Channel.IncomingPeerChannels(),
			SupportedProtocols: SupportedProtocols(),
		},
		Identity: *user2Identity,
	}

	if _, err := user2Channel.SendMessage(ctx, initiation, nil); err != nil {
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

	user1Message, err := user1PublicChannel.GetIncomingMessage(ctx, 0)
	if err != nil {
		t.Fatalf("Failed to get message : %s", err)
	}

	wMessage, err := protocols.Unwrap(user1Message.Payload())
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
		t.Fatalf("Message payload empty")
	}

	js, err := json.MarshalIndent(wMessage.Message, "", "  ")
	t.Logf("User 1 message : %s", js)

	initiation, ok := wMessage.Message.(*relationships.Initiation)
	if !ok {
		t.Fatalf("Use 1 message not initiation")
	}

	if !initiation.Configuration.PublicKey.Equal(user2Channel.Key().PublicKey()) {
		t.Errorf("Wrong public key in initiation : got %s, want %s",
			initiation.Configuration.PublicKey, user2Channel.Key().PublicKey())
	}

	if initiation.Configuration.PeerChannels[0].ID !=
		user2Channel.IncomingPeerChannels()[0].ID {
		t.Errorf("Wrong peer channel in initiation : got %s, want %s",
			initiation.Configuration.PeerChannels[0].ID,
			user2Channel.IncomingPeerChannels()[0].ID)
	}

	if err := user1Channel.InitializeRelationship(ctx, protocols,
		user1Message.Payload(), initiation.Configuration.PublicKey,
		initiation.Configuration.PeerChannels); err != nil {
		t.Fatalf("Failed to initialize channel : %s", err)
	}

	// Respond to relationship initiation
	responseInitiation := &relationships.Initiation{
		Configuration: relationships.ChannelConfiguration{
			PublicKey:          user1Channel.Key().PublicKey(),
			PeerChannels:       user1Channel.IncomingPeerChannels(),
			SupportedProtocols: SupportedProtocols(),
		},
		Identity: *user1Identity,
	}

	responseID := user1Message.ID()
	if _, err := user1Channel.SendMessage(ctx, responseInitiation, &responseID); err != nil {
		t.Fatalf("Failed to send initiation : %s", err)
	}

	/***************************** Receive Initiation Response Message ****************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	user2Messages, err := client2.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(user2Messages) != 0 {
		t.Fatalf("Wrong message count : got %d, want %d", len(user2Messages), 0)
	}

	user2Message, err := user2Channel.GetIncomingMessage(ctx, 0)
	if err != nil {
		t.Fatalf("Failed to get message : %s", err)
	}

	wMessage, err = protocols.Unwrap(user2Message.Payload())
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
		t.Fatalf("Missing relationship initiation payload")
	}

	js, err = json.MarshalIndent(wMessage.Message, "", "  ")
	t.Logf("User 2 message : %s", js)

	msg, ok := wMessage.Message.(*relationships.Initiation)
	if !ok {
		t.Fatalf("Message not relationship initiation")
	}

	publicKey := user1Channel.Key().PublicKey()

	if !msg.Configuration.PublicKey.Equal(publicKey) {
		t.Errorf("Wrong public key in initiation response : got %s, want %s",
			msg.Configuration.PublicKey, publicKey)
	}

	if msg.Configuration.PeerChannels[0].ID != user1Channel.IncomingPeerChannels()[0].ID {
		t.Errorf("Wrong peer channel in initiation response : got %s, want %s",
			msg.Configuration.PeerChannels[0].ID, user1Channel.IncomingPeerChannels()[0].ID)
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
	protocols := BuildChannelsProtocols()

	/******************************** Create User 1 Client ****************************************/
	/**********************************************************************************************/
	client1 := MockClient(ctx, store, protocols, peerChannelsFactory)
	user1PublicChannel, err := client1.CreateRelationshipInitiationChannel(ctx, wallet.RandomHash())
	if err != nil {
		t.Fatalf("Failed to create channel : %s", err)
	}

	/******************************** Create User 2 Client ****************************************/
	/**********************************************************************************************/
	client2 := MockClient(ctx, store, protocols, peerChannelsFactory)
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

	merkleProof := &merkle_proofs.MerkleProof{
		MerkleProof: fakeMerkleProof,
	}

	js, _ := json.MarshalIndent(merkleProof, "", "  ")
	t.Logf("Merkle Proof : %s", js)

	if _, err := user2Channel.SendMessage(ctx, merkleProof, nil); err != nil {
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

	wMessage, err := protocols.Unwrap(user1Messages[0].Message.Payload())
	if err != nil {
		t.Fatalf("Failed to unwrap message : %s", err)
	}

	if _, ok := wMessage.Message.(*merkle_proofs.MerkleProof); !ok {
		t.Errorf("Message not a merkle proof")
	}

	autoResponse := user1Messages[0].Message.Response()
	if len(autoResponse) == 0 {
		t.Fatalf("No auto response provided")
	}

	wMessage, err = protocols.Unwrap(autoResponse)
	if err != nil {
		t.Fatalf("Failed to unwrap message : %s", err)
	}

	if wMessage.Response == nil {
		t.Fatalf("Missing reject response")
	}

	if wMessage.Response.Status != channels.StatusReject {
		t.Fatalf("Wrong response status : got %s, want %s", wMessage.Response.Status,
			channels.StatusReject)
	}

	js, _ = json.MarshalIndent(wMessage.Response, "", "  ")
	t.Logf("Reject : %s", js)

	if wMessage.Signature != nil {
		t.Errorf("Auto response should not be signed")
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
