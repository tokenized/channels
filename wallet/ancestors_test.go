package wallet

import (
	"context"
	"testing"

	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

func Test_PopulateExpandedTx_TwoLevels(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	wallet, verifier, quoter := MockWallet()
	receiverWallet := MockWalletWith(verifier, quoter)

	utxos := MockUTXOs(ctx, wallet, 2000, 3000, 6000)

	contextID := RandomHash()
	etx, _, err := receiverWallet.CreateBitcoinReceive(ctx, contextID, 9500)
	if err != nil {
		t.Fatalf("Failed to create bitcoin receive : %s", err)
	}

	for _, utxo := range utxos {
		etx.Tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&utxo.Hash, utxo.Index), nil))
	}

	populateDepth, err := wallet.PopulateExpandedTx(ctx, etx)
	if err != nil {
		t.Fatalf("Failed to populate tx : %s", err)
	}

	t.Logf("Expanded Tx : %s", etx.String())

	depth, err := receiverWallet.VerifyExpandedTx(ctx, contextID, etx)
	if err != nil {
		t.Fatalf("Failed to verify expanded tx : %s", err)
	}
	t.Logf("Depth %d", depth)

	if populateDepth != depth {
		t.Errorf("Populate depth didn't match verify depth : populate %d, verify %d", populateDepth,
			depth)
	}

	if depth != 2 {
		t.Errorf("Wrong ancestry depth : got %d, want %d", depth, 2)
	}
}

func Test_PopulateExpandedTx_DuplicateAncestor(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	wallet, verifier, quoter := MockWallet()
	receiverWallet := MockWalletWith(verifier, quoter)

	contextID := RandomHash()
	MockReceiveTxWithProof(ctx, wallet, contextID, 12000, 20092)

	etx, _, err := receiverWallet.CreateBitcoinReceive(ctx, contextID, 25000)
	if err != nil {
		t.Fatalf("Failed to create bitcoin receive : %s", err)
	}

	populateDepth, err := wallet.FundTx(ctx, contextID, etx)
	if err != nil {
		t.Fatalf("Failed to fund tx : %s", err)
	}

	t.Logf("Expanded Tx : %s", etx.String())

	if len(etx.Tx.TxIn) != 2 {
		t.Errorf("Wrong tx input count : got %d, want %d", len(etx.Tx.TxIn), 2)
	}

	if len(etx.Ancestors) != 1 {
		t.Errorf("Wrong ancestor count : got %d, want %d", len(etx.Ancestors), 1)
	}

	depth, err := receiverWallet.VerifyExpandedTx(ctx, contextID, etx)
	if err != nil {
		t.Fatalf("Failed to verify expanded tx : %s", err)
	}
	t.Logf("Depth %d", depth)

	if populateDepth != depth {
		t.Errorf("Populate depth didn't match verify depth : populate %d, verify %d", populateDepth,
			depth)
	}

	if depth != 1 {
		t.Errorf("Wrong ancestry depth : got %d, want %d", depth, 1)
	}
}

func Test_PopulateExpandedTx_MissingInput(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	wallet, verifier, quoter := MockWallet()
	receiverWallet := MockWalletWith(verifier, quoter)

	// Create tx with two inputs that have merkle proofs, then remove one of them from the list of
	// ancestors.
	contextID := RandomHash()
	MockReceiveTxWithProof(ctx, wallet, contextID, 12000)
	MockReceiveTxWithProof(ctx, wallet, contextID, 20092)

	etx, _, err := receiverWallet.CreateBitcoinReceive(ctx, contextID, 25000)
	if err != nil {
		t.Fatalf("Failed to create bitcoin receive : %s", err)
	}

	populateDepth, err := wallet.FundTx(ctx, contextID, etx)
	if err != nil {
		t.Fatalf("Failed to fund tx : %s", err)
	}

	t.Logf("Expanded Tx : %s", etx.String())

	if len(etx.Tx.TxIn) != 2 {
		t.Errorf("Wrong tx input count : got %d, want %d", len(etx.Tx.TxIn), 2)
	}

	if len(etx.Ancestors) != 2 {
		t.Errorf("Wrong ancestor count : got %d, want %d", len(etx.Ancestors), 2)
	}

	etx.Ancestors = etx.Ancestors[:1]

	depth, err := receiverWallet.VerifyExpandedTx(ctx, contextID, etx)
	if errors.Cause(err) != MissingInput {
		t.Fatalf("Verify expanded tx returned wrong error : got %s, want %s", err, MissingInput)
	}
	t.Logf("Depth %d", depth)

	if populateDepth != depth {
		t.Errorf("Populate depth didn't match verify depth : populate %d, verify %d", populateDepth,
			depth)
	}

	if depth != 1 {
		t.Errorf("Wrong ancestry depth : got %d, want %d", depth, 1)
	}
}

func Test_PopulateExpandedTx_MissingMerkleProofAncestors(t *testing.T) {
	ctx := logger.ContextWithLogger(context.Background(), true, true, "")
	wallet, verifier, quoter := MockWallet()
	receiverWallet := MockWalletWith(verifier, quoter)

	utxos := MockUTXOs(ctx, wallet, 2000, 3000, 6000)

	contextID := RandomHash()
	etx, _, err := receiverWallet.CreateBitcoinReceive(ctx, contextID, 9500)
	if err != nil {
		t.Fatalf("Failed to create bitcoin receive : %s", err)
	}

	for _, utxo := range utxos {
		etx.Tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&utxo.Hash, utxo.Index), nil))
	}

	populateDepth, err := wallet.PopulateExpandedTx(ctx, etx)
	if err != nil {
		t.Fatalf("Failed to populate tx : %s", err)
	}

	// t.Logf("Expanded Tx : %s", etx.String())

	if len(etx.Tx.TxIn) != 3 {
		t.Errorf("Wrong tx input count : got %d, want %d", len(etx.Tx.TxIn), 3)
	}

	etx.Ancestors = etx.Ancestors[:len(etx.Ancestors)-1]

	depth, err := receiverWallet.VerifyExpandedTx(ctx, contextID, etx)
	if errors.Cause(err) != MissingMerkleProofAncestors {
		t.Fatalf("Verify expanded tx returned wrong error : got %s, want %s", err,
			MissingMerkleProofAncestors)
	}
	t.Logf("Depth %d", depth)

	if populateDepth != depth {
		t.Errorf("Populate depth didn't match verify depth : populate %d, verify %d", populateDepth,
			depth)
	}

	if depth != 2 {
		t.Errorf("Wrong ancestry depth : got %d, want %d", depth, 2)
	}
}
