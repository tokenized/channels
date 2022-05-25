package wallet

import (
	"context"
	"reflect"
	"testing"

	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/storage"

	"github.com/pkg/errors"
)

func Test_Wallet_Serialize(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	store := storage.NewMockStorage()

	config := Config{
		SatoshiBreakValue: 10000,
		BreakCount:        5,
	}

	merkleProofVerifier := NewMockMerkleProofVerifier()
	feeQuoter := NewMockFeeQuoter()

	key, err := bitcoin.GenerateKey(bitcoin.MainNet)
	if err != nil {
		t.Fatalf("Failed to generate key : %s", err)
	}

	wallet := NewWallet(config, store, merkleProofVerifier, feeQuoter, key)

	MockUTXOs(ctx, wallet, 6000, 4000, 10000)

	if err := wallet.Save(ctx); err != nil {
		t.Fatalf("Failed to save wallet : %s", err)
	}

	wrongKey, err := bitcoin.GenerateKey(bitcoin.MainNet)
	if err != nil {
		t.Fatalf("Failed to generate key : %s", err)
	}

	readWallet := NewWallet(config, store, merkleProofVerifier, feeQuoter, wrongKey)
	if err := readWallet.Load(ctx); err == nil {
		t.Fatalf("Wallet load should fail with wrong key")
	} else if errors.Cause(err) != ErrWrongKey {
		t.Fatalf("Wallet load failed with wrong error : got %s, want %s", err, ErrWrongKey)
	}

	readWallet = NewWallet(config, store, merkleProofVerifier, feeQuoter, key)
	if err := readWallet.Load(ctx); err != nil {
		t.Fatalf("Failed to load wallet : %s", err)
	}

	if len(readWallet.outputs) != len(wallet.outputs) {
		t.Fatalf("Wrong read output count : got %d, want %d", len(readWallet.outputs),
			len(wallet.outputs))
	}

	for i, output := range wallet.outputs {
		t.Logf("Output %d : %s:%d", i, output.TxID, output.Index)
	}

	for i, output := range wallet.outputs {
		t.Logf("Read output %d : %s:%d", i, readWallet.outputs[i].TxID, readWallet.outputs[i].Index)
		if !output.TxID.Equal(&readWallet.outputs[i].TxID) {
			t.Errorf("Wrong output txid at %d : got %s, want %s", i, readWallet.outputs[i].TxID,
				output.TxID)
		}
	}

	if len(readWallet.keys) != len(wallet.keys) {
		t.Errorf("Wrong read key set count : got %d, want %d", len(readWallet.keys),
			len(wallet.keys))
	}

	for contextID, keys := range wallet.keys {
		t.Logf("Keys %s : %d", contextID, len(keys))
	}

	for contextID, _ := range wallet.keys {
		t.Logf("Read keys %s : %d", contextID, len(readWallet.keys[contextID]))
	}

	if !reflect.DeepEqual(readWallet, wallet) {
		t.Errorf("Read wallet not equal")
	}
}
