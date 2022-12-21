package invoices

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"

	"github.com/tokenized/channels"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/expanded_tx"
	"github.com/tokenized/pkg/wire"

	"github.com/go-test/deep"
	"github.com/google/uuid"
)

func Test_Invoices_Menu(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)

	item1ID := uuid.New()
	item1TokenQuantity := uint64(100)
	token1Protocol := []byte("TKN")
	token1ID := uuid.New()
	bitcoinProtocol := []byte("Bitcoin")
	item1BitcoinQuantity := uint64(540000)

	msg := &Menu{
		Items: Items{
			{
				ID:          item1ID[:],
				Name:        "Item 1",
				Description: "The first item for sale",
				Prices: Prices{
					{
						Token: TokenID{
							Protocol: token1Protocol,
							ID:       token1ID[:],
						},
						Quantity: &item1TokenQuantity,
					},
					{
						Token: TokenID{
							Protocol: bitcoinProtocol,
						},
						Quantity: &item1BitcoinQuantity,
					},
				},
			},
		},
	}

	payload, err := msg.Write()
	if err != nil {
		t.Fatalf("Failed to write invoice : %s", err)
	}

	signature, err := channels.Sign(payload, key, nil, true)
	if err != nil {
		t.Fatalf("Failed to sign payload : %s", err)
	}

	signedPayload, err := signature.Wrap(payload)
	if err != nil {
		t.Fatalf("Failed to create signed payload : %s", err)
	}

	envelopeScriptItems := envelopeV1.Wrap(signedPayload)
	script, err := envelopeScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create script : %s", err)
	}

	t.Logf("Script (%d bytes) : %s", len(script), script)

	readPayload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		t.Fatalf("Failed to parse script : %s", err)
	}

	signed, signedPayload, err := channels.ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, _, err := Parse(signedPayload)
	if err != nil {
		t.Fatalf("Failed to read invoice : %s", err)
	}

	js, _ := json.MarshalIndent(readMsg, "", "  ")
	t.Logf("Invoice message : %s", js)

	if _, ok := readMsg.(*Menu); !ok {
		t.Errorf("Wrong message type")
	}

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("Unmarshalled value not equal : %v", deep.Equal(readMsg, msg))
	}
}

func Test_Invoices_Invoice(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)
	lockingScript, _ := key.LockingScript()
	unlockingScript := make(bitcoin.Script, 165)
	rand.Read(unlockingScript)

	outputs := make([]*wire.TxOut, 2)
	outputs[0] = &wire.TxOut{
		Value:         200010,
		LockingScript: lockingScript,
	}
	outputs[1] = &wire.TxOut{
		Value:         404000,
		LockingScript: lockingScript,
	}

	tx := wire.NewMsgTx(1)
	for range outputs {
		hash := &bitcoin.Hash32{}
		rand.Read((*hash)[:])
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, uint32(rand.Intn(5))), unlockingScript))
	}

	tx.AddTxOut(wire.NewTxOut(500000, lockingScript))

	item1ID := uuid.New()
	item1Quantity := uint64(10)
	bitcoinProtocol := []byte("Bitcoin")
	item1BitcoinQuantity := uint64(540000)

	data := &Invoice{
		Items: InvoiceItems{
			{
				ID: item1ID[:],
				Price: Price{
					Token: TokenID{
						Protocol: bitcoinProtocol,
					},
					Quantity: &item1BitcoinQuantity,
				},
				Quantity: &item1Quantity,
			},
		},
	}

	dataPayload, err := data.Write()
	if err != nil {
		t.Fatalf("Failed to write invoice data : %s", err)
	}

	envelopeDataScriptItems := envelopeV1.Wrap(dataPayload)
	dataScript, err := envelopeDataScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create data script : %s", err)
	}
	t.Logf("Data script (%d bytes) : %s", len(dataScript), dataScript)

	tx.AddTxOut(wire.NewTxOut(0, dataScript))
	t.Logf("Tx (%d bytes) : %s", tx.SerializeSize(), tx)

	msg := &TransferRequest{
		Tx: &expanded_tx.ExpandedTx{
			Tx: tx,
		},
	}

	payload, err := msg.Write()
	if err != nil {
		t.Fatalf("Failed to write invoice : %s", err)
	}

	signature, err := channels.Sign(payload, key, nil, true)
	if err != nil {
		t.Fatalf("Failed to sign payload : %s", err)
	}

	signedPayload, err := signature.Wrap(payload)
	if err != nil {
		t.Fatalf("Failed to create signed payload : %s", err)
	}

	envelopeScriptItems := envelopeV1.Wrap(signedPayload)
	script, err := envelopeScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create script : %s", err)
	}

	t.Logf("Invoice Script (%d bytes) : %s", len(script), script)

	readPayload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		t.Fatalf("Failed to parse script : %s", err)
	}

	signed, signedPayload, err := channels.ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, _, err := Parse(signedPayload)
	if err != nil {
		t.Fatalf("Failed to read invoice : %s", err)
	}

	js, _ := json.MarshalIndent(readMsg, "", "  ")
	t.Logf("Invoice message : %s", js)

	if _, ok := readMsg.(*TransferRequest); !ok {
		t.Errorf("Wrong message type")
	}

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("Unmarshalled value not equal : %v", deep.Equal(readMsg, msg))
	}
}
