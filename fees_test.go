package channels

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/merchant_api"
	"github.com/tokenized/pkg/wire"
)

func Test_RequiredFee(t *testing.T) {
	tests := []struct {
		name         string
		requirements FeeRequirements
		byteCounts   FeeByteCounts
		requiredFee  uint64
	}{
		{
			name: "standard only",
			requirements: FeeRequirements{
				{
					FeeType:  merchant_api.FeeTypeStandard,
					Satoshis: 500,
					Bytes:    1000,
				},
				{
					FeeType:  merchant_api.FeeTypeData,
					Satoshis: 100,
					Bytes:    1000,
				},
			},
			byteCounts: FeeByteCounts{
				{
					FeeType: merchant_api.FeeTypeStandard,
					Bytes:   100,
				},
				{
					FeeType: merchant_api.FeeTypeData,
					Bytes:   0,
				},
			},
			requiredFee: 50,
		},
		{
			name: "standard and data",
			requirements: FeeRequirements{
				{
					FeeType:  merchant_api.FeeTypeStandard,
					Satoshis: 500,
					Bytes:    1000,
				},
				{
					FeeType:  merchant_api.FeeTypeData,
					Satoshis: 100,
					Bytes:    1000,
				},
			},
			byteCounts: FeeByteCounts{
				{
					FeeType: merchant_api.FeeTypeStandard,
					Bytes:   100,
				},
				{
					FeeType: merchant_api.FeeTypeData,
					Bytes:   50,
				},
			},
			requiredFee: 55,
		},
		{
			name: "lots of data",
			requirements: FeeRequirements{
				{
					FeeType:  merchant_api.FeeTypeStandard,
					Satoshis: 500,
					Bytes:    1000,
				},
				{
					FeeType:  merchant_api.FeeTypeData,
					Satoshis: 100,
					Bytes:    1000,
				},
			},
			byteCounts: FeeByteCounts{
				{
					FeeType: merchant_api.FeeTypeStandard,
					Bytes:   100,
				},
				{
					FeeType: merchant_api.FeeTypeData,
					Bytes:   500000,
				},
			},
			requiredFee: 50050,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requiredFee := tt.requirements.RequiredFee(tt.byteCounts)
			t.Logf("Required fee : %d", requiredFee)
			if requiredFee != tt.requiredFee {
				t.Errorf("Wrong required fee : got %d, want %d", requiredFee, tt.requiredFee)
			}
		})
	}
}

func Test_FeeByteCounts(t *testing.T) {
	var inputHash bitcoin.Hash32
	rand.Read(inputHash[:])

	standardTx := wire.NewMsgTx(1)
	standardTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&inputHash, 8), make([]byte, 100)))
	standardTx.AddTxOut(wire.NewTxOut(50000, make([]byte, 50)))

	standardTxSize := uint64(standardTx.SerializeSize())

	dataTx := wire.NewMsgTx(1)
	dataTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&inputHash, 8), make([]byte, 100)))
	dataScript := make([]byte, 500)
	dataScript[0] = bitcoin.OP_FALSE
	dataScript[1] = bitcoin.OP_RETURN
	dataTx.AddTxOut(wire.NewTxOut(50000, dataScript))

	dataTxSize := uint64(dataTx.SerializeSize())
	dataSize := uint64(len(dataScript))

	tests := []struct {
		name       string
		tx         *wire.MsgTx
		byteCounts FeeByteCounts
	}{
		{
			name: "standard only",
			tx:   standardTx,
			byteCounts: FeeByteCounts{
				{
					FeeType: merchant_api.FeeTypeStandard,
					Bytes:   standardTxSize,
				},
				{
					FeeType: merchant_api.FeeTypeData,
					Bytes:   0,
				},
			},
		},
		{
			name: "data",
			tx:   dataTx,
			byteCounts: FeeByteCounts{
				{
					FeeType: merchant_api.FeeTypeStandard,
					Bytes:   dataTxSize - dataSize,
				},
				{
					FeeType: merchant_api.FeeTypeData,
					Bytes:   dataSize,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			byteCounts := TxFeeByteCounts(tt.tx)
			js, _ := json.Marshal(byteCounts)
			t.Logf("Byte Counts : %s", js)

			if len(byteCounts) != len(tt.byteCounts) {
				t.Fatalf("Wrong byte counts length : got %d, want %d", len(byteCounts),
					len(tt.byteCounts))
			}

			for i := range byteCounts {
				if byteCounts[i].FeeType != tt.byteCounts[i].FeeType {
					t.Errorf("Wrong fee type %d : got %s, want %s", i, byteCounts[i].FeeType,
						tt.byteCounts[i].FeeType)
				}

				if byteCounts[i].Bytes != tt.byteCounts[i].Bytes {
					t.Errorf("Wrong byte count %d : got %d, want %d", i, byteCounts[i].Bytes,
						tt.byteCounts[i].Bytes)
				}
			}
		})
	}
}
