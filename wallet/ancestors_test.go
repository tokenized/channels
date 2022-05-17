package wallet

import (
	"context"
	"testing"

	"github.com/tokenized/pkg/logger"
)

func Test_PopulateExpandedTx_TwoLevels(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	wallet, verifier, quoter := MockWallet()
	receiverWallet := MockWalletWith(verifier, quoter)

	MockUTXOs(ctx, wallet, 6000, 4000, 10000)

	contextID := RandomHash()
	etx, err := receiverWallet.CreateBitcoinReceive(ctx, contextID, 9500)
	if err != nil {
		t.Fatalf("Failed to create bitcoin receive : %s", err)
	}

	if err := wallet.FundTx(ctx, contextID, etx); err != nil {
		t.Fatalf("Failed to fund tx : %s", err)
	}

	t.Logf("Expanded Tx : %s", etx.String())

	if err := receiverWallet.VerifyExpandedTx(ctx, etx); err != nil {
		t.Fatalf("Failed to verify expanded tx : %s", err)
	}
}

func Test_PopulateExpandedTx_DuplicateAncestor(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	wallet, verifier, quoter := MockWallet()
	receiverWallet := MockWalletWith(verifier, quoter)

	contextID := RandomHash()
	MockReceiveTx(ctx, wallet, contextID, 12000, 34092)

	etx, err := receiverWallet.CreateBitcoinReceive(ctx, contextID, 25000)
	if err != nil {
		t.Fatalf("Failed to create bitcoin receive : %s", err)
	}

	if err := wallet.FundTx(ctx, contextID, etx); err != nil {
		t.Fatalf("Failed to fund tx : %s", err)
	}

	t.Logf("Expanded Tx : %s", etx.String())

	if len(etx.Tx.TxIn) != 2 {
		t.Errorf("Wrong tx input count : got %d, want %d", len(etx.Tx.TxIn), 2)
	}

	if len(etx.Ancestors) != 1 {
		t.Errorf("Wrong ancestor count : got %d, want %d", len(etx.Ancestors), 1)
	}

	if err := receiverWallet.VerifyExpandedTx(ctx, etx); err != nil {
		t.Fatalf("Failed to verify expanded tx : %s", err)
	}
}
