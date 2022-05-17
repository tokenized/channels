package wallet

import (
	"context"
	"testing"

	"github.com/tokenized/pkg/logger"
)

func Test_PopulateExpandedTx(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	wallet, verifier, quoter := MockWallet()
	receiverWallet := MockWalletWith(verifier, quoter)

	GenerateUTXOsWithProofs(ctx, wallet, 53000, 140392)

	contextID := RandomHash()
	etx, err := receiverWallet.CreateBitcoinReceive(ctx, contextID, 95000)
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
