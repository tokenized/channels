package wallet

import (
	"context"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

// PopulateExpandedTx adds any ancestors that are needed for the tx. This should be called after
// funding the tx.
func (w *Wallet) PopulateExpandedTx(ctx context.Context, etx *channels.ExpandedTx) error {
	for _, txin := range etx.Tx.TxIn {
		if etx.Ancestors.GetTx(txin.PreviousOutPoint.Hash) != nil {
			continue // tx already in ancestors
		}

		if err := w.addAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash); err != nil {
			return errors.Wrap(err, txin.PreviousOutPoint.Hash.String())
		}
	}

	return nil
}

func (w *Wallet) addAncestorTx(ctx context.Context, etx *channels.ExpandedTx,
	txid bitcoin.Hash32) error {

	tx, err := fetchTx(ctx, w.store, txid)
	if err != nil {
		return errors.Wrap(err, "fetch tx")
	}
	if tx == nil {
		return nil
	}

	ancestor := &channels.AncestorTx{}

	proof, err := tx.GetMerkleProof(ctx, w.merkleProofVerifier)
	if err != nil {
		return errors.Wrap(err, "tx merkle proof")
	}
	if proof != nil {
		proof.Tx = tx.Tx // embed tx in merkle proof
		ancestor.MerkleProof = proof
	} else {
		ancestor.Tx = tx.Tx
	}

	etx.Ancestors = append(etx.Ancestors, ancestor)

	// Check inputs of that tx
	for _, txin := range tx.Tx.TxIn {
		if etx.Ancestors.GetTx(txin.PreviousOutPoint.Hash) != nil {
			continue // tx already in ancestors
		}

		if err := w.addAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash); err != nil {
			return errors.Wrap(err, txin.PreviousOutPoint.Hash.String())
		}
	}

	return nil
}

func (w *Wallet) VerifyExpandedTx(ctx context.Context, etx *channels.ExpandedTx) error {
	verified := make(map[bitcoin.Hash32]bool)
	for _, txin := range etx.Tx.TxIn {
		if err := w.verifyAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash, &verified); err != nil {
			return errors.Wrap(err, txin.PreviousOutPoint.Hash.String())
		}
	}

	return nil
}

func (w *Wallet) verifyAncestorTx(ctx context.Context, etx *channels.ExpandedTx,
	txid bitcoin.Hash32, verified *map[bitcoin.Hash32]bool) error {

	if _, exists := (*verified)[txid]; exists {
		return nil // already verified this tx
	}

	atx := etx.Ancestors.GetTx(txid)
	if atx == nil {
		return errors.Wrap(ErrMissingAncestor, txid.String())
	}

	if atx.MerkleProof != nil {
		_, isLongest, err := w.merkleProofVerifier.VerifyMerkleProof(ctx, atx.MerkleProof)
		if err != nil {
			return errors.Wrapf(err, "merkle proof: %s", txid)
		}

		if isLongest {
			(*verified)[txid] = true
			return nil
		}

		return errors.Wrap(ErrNotMostPOW, txid.String())
	}

	tx := atx.GetTx()
	if tx == nil {
		return errors.Wrap(ErrMissingAncestor, txid.String())
	}
	for _, txin := range tx.TxIn {
		if err := w.verifyAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash, verified); err != nil {
			return errors.Wrap(err, txid.String())
		}
	}

	(*verified)[txid] = true
	return nil
}
