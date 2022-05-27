package client

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/tokenized/channels"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/merchant_api"
	"github.com/tokenized/pkg/peer_channels"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/txbuilder"
	"github.com/tokenized/pkg/wire"
)

func Test_Invoice(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	store := storage.NewMockStorage()
	peerChannelsFactory := peer_channels.NewFactory()

	merchant, buyer := MockRelatedUsers(ctx, store, peerChannelsFactory)

	merchantChannel := merchant.Channel(0)

	buyerChannel := buyer.Channel(0)

	/*************************************** Start Clients ****************************************/
	/**********************************************************************************************/

	wait := &sync.WaitGroup{}

	interruptA := make(chan interface{})
	wait.Add(1)
	go func() {
		merchant.Run(ctx, interruptA)
		wait.Done()
	}()

	interruptB := make(chan interface{})
	wait.Add(1)
	go func() {
		buyer.Run(ctx, interruptB)
		wait.Done()
	}()

	/*************************************** Send Invoice *****************************************/
	/**********************************************************************************************/

	one := uint64(1)
	fiftyK := uint64(50000)
	invoice := &channels.Invoice{
		Items: channels.InvoiceItems{
			{
				ID: []byte("Service A"),
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
		Tx: &channels.ExpandedTx{
			Tx: tx,
		},
		Fees: channels.FeeRequirements{
			{
				FeeType:  merchant_api.FeeTypeStandard,
				Satoshis: 500,
				Bytes:    1000,
			},
			{
				FeeType:  merchant_api.FeeTypeStandard,
				Satoshis: 250,
				Bytes:    1000,
			},
		},
	}

	js, _ := json.MarshalIndent(invoiceTx, "", "  ")
	t.Logf("Invoice Tx : %s", js)

	if err := merchantChannel.SendMessage(ctx, invoiceTx, nil); err != nil {
		t.Fatalf("Failed to send invoice : %s", err)
	}

	time.Sleep(250 * time.Millisecond)

	/**************************************** Send Payment ****************************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	buyerMessages, err := buyer.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(buyerMessages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(buyerMessages), 1)
	}

	wMessage, err := channels.Unwrap(buyerMessages[0].Message.Payload())
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

	// Payment
	if err := txb.AddInput(*wire.NewOutPoint(previousTx.TxHash(), 0), paymentLockingScript,
		125000); err != nil {
		t.Fatalf("Failed to add payment input : %s", err)
	}

	// Change
	if err := txb.AddOutput(paymentLockingScript, 75000, true, false); err != nil {
		t.Fatalf("Failed to add change output : %s", err)
	}

	fakeMerkleProof := MockMerkleProof(previousTx)

	payment := &channels.InvoicePayment{
		Tx: &channels.ExpandedTx{
			Tx: txb.MsgTx,
			Ancestors: channels.AncestorTxs{
				{
					MerkleProof: fakeMerkleProof,
				},
			},
		},
	}

	js, _ = json.MarshalIndent(payment, "", "  ")
	t.Logf("Invoice Payment : %s", js)

	t.Logf("Invoice tx id : %d", buyerMessages[0].Message.ID())

	responseID := buyerMessages[0].Message.ID()
	if err := buyerChannel.SendMessage(ctx, payment, &responseID); err != nil {
		t.Fatalf("Failed to send invoice payment : %s", err)
	}

	time.Sleep(250 * time.Millisecond)

	/*************************************** Accept Payment ***************************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	merchantMessages, err := merchant.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(merchantMessages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(merchantMessages), 1)
	}

	wMessage, err = channels.Unwrap(merchantMessages[0].Message.Payload())
	if err != nil {
		t.Fatalf("Failed to unwrap message : %s", err)
	}

	if wMessage.Response == nil {
		t.Fatalf("Payment is not a response")
	}

	receivedInvoicePayment, ok := wMessage.Message.(*channels.InvoicePayment)
	if !ok {
		t.Fatalf("Received message not an invoice payment")
	}

	js, _ = json.MarshalIndent(receivedInvoicePayment, "", "  ")
	t.Logf("Received Invoice Payment : %s", js)

	paymentAccept := &channels.InvoicePaymentAccept{}

	js, _ = json.MarshalIndent(paymentAccept, "", "  ")
	t.Logf("Invoice Payment Accept : %s", js)

	responseID = merchantMessages[0].Message.ID()
	if err := merchantChannel.SendMessage(ctx, paymentAccept, &responseID); err != nil {
		t.Fatalf("Failed to send invoice payment accept : %s", err)
	}

	time.Sleep(250 * time.Millisecond)

	/*********************************** Receive Payment Accept ***********************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	buyerMessages, err = buyer.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(buyerMessages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(buyerMessages), 1)
	}

	wMessage, err = channels.Unwrap(buyerMessages[0].Message.Payload())
	if err != nil {
		t.Fatalf("Failed to unwrap message : %s", err)
	}

	if wMessage.Response == nil {
		t.Fatalf("Payment accept is not a response")
	}

	receivedPaymentAccept, ok := wMessage.Message.(*channels.InvoicePaymentAccept)
	if !ok {
		t.Fatalf("Received message not a payment accept")
	}

	js, _ = json.MarshalIndent(receivedPaymentAccept, "", "  ")
	t.Logf("Received Payment Accept : %s", js)

	if err := buyerMessages[0].Channel.MarkMessageProcessed(ctx,
		buyerMessages[0].Message.ID()); err != nil {
		t.Fatalf("Failed to mark message as processed : %s", err)
	}

	/************************************** Send Merkle Proof *************************************/
	/**********************************************************************************************/

	fakeMerkleProof = MockMerkleProof(txb.MsgTx)

	merkleProof := &channels.MerkleProof{
		MerkleProof: fakeMerkleProof,
	}

	js, _ = json.MarshalIndent(merkleProof, "", "  ")
	t.Logf("Merkle Proof : %s", js)

	if err := merchantChannel.SendMessage(ctx, merkleProof, nil); err != nil {
		t.Fatalf("Failed to send Merkle Proof : %s", err)
	}

	time.Sleep(250 * time.Millisecond)

	/************************************ Receive Merkle Proof ************************************/
	/**********************************************************************************************/

	time.Sleep(time.Millisecond * 250)

	buyerMessages, err = buyer.GetUnprocessedMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get unprocessed messages : %s", err)
	}

	if len(buyerMessages) != 1 {
		t.Fatalf("Wrong message count : got %d, want %d", len(buyerMessages), 1)
	}

	wMessage, err = channels.Unwrap(buyerMessages[0].Message.Payload())
	if err != nil {
		t.Fatalf("Failed to unwrap message : %s", err)
	}

	if wMessage.Response != nil {
		t.Fatalf("Merkle proof should not be a response")
	}

	receivedMerkleProof, ok := wMessage.Message.(*channels.MerkleProof)
	if !ok {
		t.Fatalf("Received message not a merkle proof")
	}

	js, _ = json.MarshalIndent(receivedMerkleProof, "", "  ")
	t.Logf("Received Merkle Proof : %s", js)

	if err := buyerMessages[0].Channel.MarkMessageProcessed(ctx,
		buyerMessages[0].Message.ID()); err != nil {
		t.Fatalf("Failed to mark message as processed : %s", err)
	}

	/**************************************** Stop Clients ****************************************/
	/**********************************************************************************************/

	close(interruptA)
	close(interruptB)
	wait.Wait()
}
