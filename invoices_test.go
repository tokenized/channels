package channels

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"

	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
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

	vendorID := uuid.New()
	vendorKey, _ := bitcoin.GenerateKey(bitcoin.MainNet)
	vendorPublicKey := vendorKey.PublicKey()
	vendorName := "Vendor 1"

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
		Vendor: &Identity{
			ID:        vendorID[:],
			PublicKey: vendorPublicKey,
			Name:      &vendorName,
		},
	}

	protocolIDs, scriptItems, err := WriteInvoice(msg)
	if err != nil {
		t.Fatalf("Failed to write invoice : %s", err)
	}

	signedProtocolIDs, signedScriptItems, err := Sign(protocolIDs, scriptItems, key, true)

	envelopeScriptItems := envelopeV1.Wrap(signedProtocolIDs, signedScriptItems)

	script, err := envelopeScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create script : %s", err)
	}

	t.Logf("Script (%d bytes) : %s", len(script), script)

	readProtocolIDs, readPayload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		t.Fatalf("Failed to parse script : %s", err)
	}

	signed, signedProtocolIDs, signedPayload, err := ParseSigned(readProtocolIDs, readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, err := ParseInvoice(signedProtocolIDs, signedPayload)
	if err != nil {
		t.Fatalf("Failed to read invoice : %s", err)
	}

	js, _ := json.MarshalIndent(readMsg, "", "  ")
	t.Logf("Invoice message : %s", js)

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("Unmarshalled value not equal : %v", deep.Equal(readMsg, msg))
	}
}

func Test_Invoices_Invoice(t *testing.T) {
	key, _ := bitcoin.GenerateKey(bitcoin.MainNet)
	lockingScript, _ := key.LockingScript()
	unlockingScript := make(bitcoin.Script, 165)
	rand.Read(unlockingScript)

	outputs := make([]*TxOut, 2)
	outputs[0] = &TxOut{200010, lockingScript}
	outputs[1] = &TxOut{404000, lockingScript}

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
				ItemID:          item1ID[:],
				ItemDescription: "Item 1",
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

	dataProtocolIDs, dataScriptItems, err := WriteInvoice(data)
	if err != nil {
		t.Fatalf("Failed to write invoice data : %s", err)
	}

	envelopeDataScriptItems := envelopeV1.Wrap(dataProtocolIDs, dataScriptItems)
	dataScript, err := envelopeDataScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create data script : %s", err)
	}
	t.Logf("Data script (%d bytes) : %s", len(dataScript), dataScript)

	tx.AddTxOut(wire.NewTxOut(0, dataScript))
	t.Logf("Tx (%d bytes) : %s", tx.SerializeSize(), tx)

	msg := &InvoiceTx{
		Tx: ExpandedTx{
			Tx:      tx,
			Outputs: outputs,
		},
	}

	protocolIDs, scriptItems, err := WriteInvoice(msg)
	if err != nil {
		t.Fatalf("Failed to write invoice : %s", err)
	}

	signedProtocolIDs, signedScriptItems, err := Sign(protocolIDs, scriptItems, key, true)

	envelopeScriptItems := envelopeV1.Wrap(signedProtocolIDs, signedScriptItems)

	script, err := envelopeScriptItems.Script()
	if err != nil {
		t.Fatalf("Failed to create script : %s", err)
	}

	t.Logf("Invoice Script (%d bytes) : %s", len(script), script)

	readProtocolIDs, readPayload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		t.Fatalf("Failed to parse script : %s", err)
	}

	signed, signedProtocolIDs, signedPayload, err := ParseSigned(readProtocolIDs, readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, err := ParseInvoice(signedProtocolIDs, signedPayload)
	if err != nil {
		t.Fatalf("Failed to read invoice : %s", err)
	}

	js, _ := json.MarshalIndent(readMsg, "", "  ")
	t.Logf("Invoice message : %s", js)

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("Unmarshalled value not equal : %v", deep.Equal(readMsg, msg))
	}
}
