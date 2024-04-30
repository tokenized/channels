package uuid_response_handler

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/tokenized/channels"
	"github.com/tokenized/channels/peer_channels_listener"
	"github.com/tokenized/logger"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/peer_channels"

	"github.com/google/uuid"
)

func Test_MessageHandling(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")

	peerChannelsClient := peer_channels.NewMockClient()
	peerChannelsAccount, _ := peerChannelsClient.CreateAccount(ctx)
	peerChannelsAccountClient := peer_channels.NewMockAccountClient(peerChannelsClient,
		peerChannelsAccount.AccountID, peerChannelsAccount.Token)
	peerChannel, _ := peerChannelsAccountClient.CreatePublicChannel(ctx)

	handler := NewHandler()
	listener := peer_channels_listener.NewPeerChannelsListener(peerChannelsClient,
		peerChannelsAccount.Token, 100, 1, time.Second, handler.HandleMessage, handler.HandleUpdate)
	handler.SetAddUpdate(listener.AddUpdate)

	listenerInterrupt := make(chan interface{})
	listenerComplete := make(chan interface{})
	var listenerError error
	go func() {
		listenerError = listener.Run(ctx, listenerInterrupt)
		close(listenerComplete)
	}()

	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)
	id := uuid.New()

	signature := channels.NewSignature(key, channels.RandomHashPtr(), false)
	uuid := channels.UUID(id)
	response := &channels.Response{
		Status: channels.StatusOK,
		Note:   "Complete",
	}
	payload, err := channels.Wrap(nil, response, &uuid, signature)
	if err != nil {
		t.Fatalf("Failed to wrap message : %s", err)
	}

	responseChannel := handler.RegisterForResponse(peerChannel.ID, id)

	waitComplete := make(chan interface{})
	var waitErr error
	var message *peer_channels.Message
	go func() {
		message, waitErr = WaitWithTimeout(responseChannel, time.Second)
		t.Logf("Message received")
		close(waitComplete)
	}()

	if err := peerChannelsClient.WriteMessage(ctx, peerChannel.ID, peerChannel.WriteToken,
		peer_channels.ContentTypeBinary, bytes.NewReader(payload)); err != nil {
		t.Fatalf("Failed to write peer channel message : %s", err)
	}
	t.Logf("Message sent")

	select {
	case <-waitComplete:
	case <-time.After(time.Second):
		t.Fatalf("Wait for response timed out")
	}

	if waitErr != nil {
		t.Fatalf("Failed to wait for response : %s", err)
	}

	if message == nil {
		t.Fatalf("Message is nil")
	}

	if message.ChannelID != peerChannel.ID {
		t.Fatalf("Wrong message channel ID : got %s, want %s", message.ChannelID,
			peerChannel.ID)
	}

	if !bytes.Equal(message.Payload, payload) {
		t.Fatalf("Payload doesn't match")
	}

	close(listenerInterrupt)
	select {
	case <-listenerComplete:
	case <-time.After(time.Second):
		t.Fatalf("Listener shutdown timed out")
	}

	if listenerError != nil {
		t.Fatalf("Listener completed with error : %s", listenerError)
	}
}
