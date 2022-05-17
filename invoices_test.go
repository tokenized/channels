package channels

import (
	"bytes"
	"encoding/json"
	"fmt"
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
			Name: &vendorName,
		},
	}

	payload, err := msg.Write()
	if err != nil {
		t.Fatalf("Failed to write invoice : %s", err)
	}

	signature, err := Sign(payload, key, nil, true)
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

	signed, signedPayload, err := ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, err := ParseInvoice(signedPayload)
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

	msg := &InvoiceTx{
		Tx: ExpandedTx{
			Tx: tx,
		},
	}

	payload, err := msg.Write()
	if err != nil {
		t.Fatalf("Failed to write invoice : %s", err)
	}

	signature, err := Sign(payload, key, nil, true)
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

	signed, signedPayload, err := ParseSigned(readPayload)
	if err != nil {
		t.Fatalf("Failed to read signed message : %s", err)
	}

	if err := signed.Verify(); err != nil {
		t.Errorf("Failed to verify signed message : %s", err)
	} else {
		t.Logf("Verified signed message")
	}

	readMsg, err := ParseInvoice(signedPayload)
	if err != nil {
		t.Fatalf("Failed to read invoice : %s", err)
	}

	js, _ := json.MarshalIndent(readMsg, "", "  ")
	t.Logf("Invoice message : %s", js)

	if _, ok := readMsg.(*InvoiceTx); !ok {
		t.Errorf("Wrong message type")
	}

	if !reflect.DeepEqual(msg, readMsg) {
		t.Errorf("Unmarshalled value not equal : %v", deep.Equal(readMsg, msg))
	}
}

func Test_Period(t *testing.T) {
	tests := []struct {
		s string
		v Period
	}{
		{
			s: "",
			v: Period{
				Count: 0,
				Type:  PeriodTypeUnspecified,
			},
		},
		{
			s: "5 days",
			v: Period{
				Count: 5,
				Type:  PeriodTypeDay,
			},
		},
		{
			s: "week",
			v: Period{
				Count: 1,
				Type:  PeriodTypeWeek,
			},
		},
		{
			s: "month",
			v: Period{
				Count: 1,
				Type:  PeriodTypeMonth,
			},
		},
		{
			s: "2 months",
			v: Period{
				Count: 2,
				Type:  PeriodTypeMonth,
			},
		},
		{
			s: "2 years",
			v: Period{
				Count: 2,
				Type:  PeriodTypeYear,
			},
		},
		{
			s: "100 seconds",
			v: Period{
				Count: 100,
				Type:  PeriodTypeSecond,
			},
		},
		{
			s: "1000 minutes",
			v: Period{
				Count: 1000,
				Type:  PeriodTypeMinute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			t.Logf("String : %s", tt.v.String())
			if tt.v.String() != tt.s {
				t.Errorf("Wrong string value : got %s, want %s", tt.v.String(), tt.s)
			}

			js, err := json.Marshal(tt.v)
			if err != nil {
				t.Fatalf("Failed to marshal json : %s", err)
			}
			t.Logf("JSON : %s", js)

			if tt.v.Type == PeriodTypeUnspecified {
				if string(js) != "null" {
					t.Errorf("Wrong JSON : got %s, want %s", js, "null")
				}
			} else if string(js) != fmt.Sprintf("\"%s\"", tt.s) {
				t.Errorf("Wrong JSON : got %s, want %s", js, fmt.Sprintf("\"%s\"", tt.s))
			}

			v := &Period{}
			if err := json.Unmarshal(js, v); err != nil {
				t.Fatalf("Failed to unmarshal json : %s", err)
			}

			if v.Count != tt.v.Count {
				t.Errorf("Wrong json unmarshal count : got %d, want %d", v.Count, tt.v.Count)
			}
			if v.Type != tt.v.Type {
				t.Errorf("Wrong json unmarshal type : got %d, want %d", v.Type, tt.v.Type)
			}

			b, err := tt.v.MarshalBinary()
			if err != nil {
				t.Fatalf("Failed to marshal binary : %s", err)
			}
			t.Logf("Binary : 0x%x", b)

			v = &Period{}
			if err := v.UnmarshalBinary(b); err != nil {
				t.Fatalf("Failed to unmarshal binary : %s", err)
			}

			if v.Count != tt.v.Count {
				t.Errorf("Wrong binary unmarshal count : got %d, want %d", v.Count, tt.v.Count)
			}
			if v.Type != tt.v.Type {
				t.Errorf("Wrong binary unmarshal type : got %d, want %d", v.Type, tt.v.Type)
			}
		})
	}
}
