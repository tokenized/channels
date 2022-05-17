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

	if len(readWallet.Outputs) != len(wallet.Outputs) {
		t.Errorf("Wrong read output count : got %d, want %d", len(readWallet.Outputs),
			len(wallet.Outputs))
	}

	if len(readWallet.KeySet) != len(wallet.KeySet) {
		t.Errorf("Wrong read key set count : got %d, want %d", len(readWallet.KeySet),
			len(wallet.KeySet))
	}

	if !reflect.DeepEqual(readWallet, wallet) {
		t.Errorf("Read wallet not equal")
	}
}
