package client

import (
	"context"
	"crypto/sha256"
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
	"github.com/tokenized/pkg/txbuilder"
	"github.com/tokenized/pkg/wire"
)

func Test_Invoice(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	peerChannelsFactory := peer_channels.NewFactory()

	userA, userB := MockRelatedUsers(ctx, peerChannelsFactory)

	userAChannel := userA.Client.Channels[0]
	userAChannelKey := userA.HashKey(userAChannel.Hash())

	userBChannel := userB.Client.Channels[0]
	userBChannelKey := userB.HashKey(userBChannel.Hash())

	/*************************************** Start Clients ****************************************/
	/**********************************************************************************************/

	wait := &sync.WaitGroup{}

	interruptA := make(chan interface{})
	wait.Add(1)
	go func() {
		userA.Client.Run(ctx, interruptA)
		wait.Done()
	}()

	interruptB := make(chan interface{})
	wait.Add(1)
	go func() {
		userB.Client.Run(ctx, interruptB)
		wait.Done()
	}()

	/*************************************** Send Invoice *****************************************/
	/**********************************************************************************************/

	one := uint64(1)
	fiftyK := uint64(50000)
	invoice := &channels.Invoice{
		Items: channels.InvoiceItems{
			{
				ItemID: []byte("Service A"),
				Price: channels.Price{
					Quantity: &fiftyK,
				},
				Quantity: &one,
			},
		},
		Timestamp:  channels.Now(),
		Expiration: channels.ConvertToTimestamp(time.Now().Add(time.Hour)),
	}

	invoicePayload, err := invoice.Write()
	if err != nil {
		t.Fatalf("Failed to write invoice : %s", err)
	}

	invoiceScriptItems := envelopeV1.Wrap(invoicePayload)
	invoiceScript, err := invoiceScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create invoice script : %s", err)
	}

	tx := wire.NewMsgTx(1)
	key, err := bitcoin.GenerateKey(bitcoin.MainNet)
	if err != nil {
		t.Fatalf("Failed to generate key : %s", err)
	}

	lockingScript, err := key.LockingScript()
	if err != nil {
		t.Fatalf("Failed to create locking script : %s", err)
	}

	tx.AddTxOut(wire.NewTxOut(50000, lockingScript))
	tx.AddTxOut(wire.NewTxOut(0, invoiceScript))

	t.Logf("Invoice tx : %s", tx.String())

	invoiceTx := &channels.InvoiceTx{
		Tx: channels.ExpandedTx{
			Tx: tx,
		},
		Fees: channels.FeeQuotes{
			{
				FeeType: channels.FeeQuoteTypeStandard,
				MiningFee: channels.Fee{
					Satoshis: 500,
					Bytes:    1000,
				},
			},
		},
	}

	js, _ := json.MarshalIndent(invoiceTx, "", "  ")
	t.Logf("Invoice Tx : %s", js)

	msgHash := wallet.GenerateHash("invoice_tx")
	invoiceTxScript, err := channels.Wrap(invoiceTx, userAChannelKey, msgHash, nil)
	if err != nil {
		t.Fatalf("Failed to wrap invoice tx : %s", err)
	}

	t.Logf("Invoice script : %s", invoiceTxScript)

	if err := SendMessage(ctx, peerChannelsFactory, userAChannel.Outgoing.Entity.PeerChannels,
		invoiceTxScript); err != nil {
		t.Fatalf("Failed to send invoice : %s", err)
	}

	userAChannel.Outgoing.AddMessage(ctx, invoiceTxScript)

	time.Sleep(250 * time.Millisecond)

	/************************************** Receive Invoice ***************************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	userBMessages, err := userB.Client.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(userBMessages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(userBMessages), 1)
	}

	wMessage, err := channels.Unwrap(userBMessages[0].Message.Payload)
	if err != nil {
		t.Fatalf("Failed to unwrap message : %s", err)
	}

	receivedInvoiceTx, ok := wMessage.Message.(*channels.InvoiceTx)
	if !ok {
		t.Fatalf("Received message not an invoice tx")
	}

	js, _ = json.MarshalIndent(receivedInvoiceTx, "", "  ")
	t.Logf("Received Invoice Tx : %s", js)

	paymentKey, err := bitcoin.GenerateKey(bitcoin.MainNet)
	if err != nil {
		t.Fatalf("Failed to generate payment key : %s", err)
	}

	paymentLockingScript, err := paymentKey.LockingScript()
	if err != nil {
		t.Fatalf("Failed to create locking script : %s", err)
	}

	txb, err := txbuilder.NewTxBuilderFromWire(0.5, 0.25, receivedInvoiceTx.Tx.Tx, nil)
	if err != nil {
		t.Fatalf("Failed to import tx into txbuilder : %s", err)
	}

	previousTx := wire.NewMsgTx(1)
	previousTx.AddTxOut(wire.NewTxOut(125000, paymentLockingScript))

	if err := txb.AddInput(*wire.NewOutPoint(previousTx.TxHash(), 0), paymentLockingScript, 125000); err != nil {
		t.Fatalf("Failed to add payment input : %s", err)
	}

	fakeMerkleProof := MockMerkleProof(*previousTx.TxHash())

	payment := &channels.InvoicePayment{
		Tx: channels.ExpandedTx{
			Tx: txb.MsgTx,
			Ancestors: channels.AncestorTxs{
				{
					Tx:          previousTx,
					MerkleProof: fakeMerkleProof,
				},
			},
		},
	}

	js, _ = json.MarshalIndent(payment, "", "  ")
	t.Logf("Invoice Payment : %s", js)

	responseHash := bitcoin.Hash32(sha256.Sum256(userBMessages[0].Message.Payload))

	msgHash = wallet.GenerateHash("invoice_tx")
	paymentScript, err := channels.Wrap(payment, userBChannelKey, msgHash, &responseHash)
	if err != nil {
		t.Fatalf("Failed to wrap invoice tx : %s", err)
	}

	t.Logf("Payment script : %s", paymentScript)

	if err := SendMessage(ctx, peerChannelsFactory, userBChannel.Outgoing.Entity.PeerChannels,
		paymentScript); err != nil {
		t.Fatalf("Failed to send invoice : %s", err)
	}

	userAChannel.Outgoing.AddMessage(ctx, paymentScript)

	time.Sleep(250 * time.Millisecond)

	/**************************************** Stop Clients ****************************************/
	/**********************************************************************************************/

	close(interruptA)
	close(interruptB)
	wait.Wait()
}
