package wallet

import (
	"context"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"

	"github.com/pkg/errors"
)

// PopulateExpandedTx adds any ancestors that are needed for the tx. This should be called after
// funding the tx. It returns the depth of ancestry provided. It can also return MissingInputs or
// MissingMerkleProofAncestors which don't mean failure, but notify that full ancestry back to
// merkle proofs were not available.
func (w *Wallet) PopulateExpandedTx(ctx context.Context, etx *channels.ExpandedTx) (uint, error) {
	highestDepth := uint(0)
	missingInputs := false
	missingMerkleProofAncestors := false
	for _, txin := range etx.Tx.TxIn {
		if etx.Ancestors.GetTx(txin.PreviousOutPoint.Hash) != nil {
			continue // tx already in ancestors
		}

		depth, err := w.addAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash, 0)
		if err != nil {
			if errors.Cause(err) != MissingMerkleProofAncestors {
				return 0, errors.Wrap(err, txin.PreviousOutPoint.Hash.String())
			}

			if depth == 0 {
				missingInputs = true
			}

			missingMerkleProofAncestors = true
		}

		if depth > highestDepth {
			highestDepth = depth
		}
	}

	if missingInputs {
		return highestDepth, MissingInput
	}

	if missingMerkleProofAncestors {
		return highestDepth, MissingMerkleProofAncestors
	}

	return highestDepth, nil
}

func (w *Wallet) addAncestorTx(ctx context.Context, etx *channels.ExpandedTx, txid bitcoin.Hash32,
	depth uint) (uint, error) {

	tx, err := fetchTx(ctx, w.store, txid)
	if err != nil {
		return 0, errors.Wrap(err, "fetch tx")
	}
	if tx == nil {
		return depth, MissingMerkleProofAncestors
	}

	ancestor := &channels.AncestorTx{}
	etx.Ancestors = append(etx.Ancestors, ancestor)

	proof, err := tx.GetMerkleProof(ctx, w.merkleProofVerifier)
	if err != nil {
		return 0, errors.Wrap(err, "tx merkle proof")
	}
	if proof != nil {
		proof.Tx = tx.Tx // embed tx in merkle proof
		ancestor.MerkleProof = proof
		return depth + 1, nil
	} else {
		ancestor.Tx = tx.Tx // embed tx directly
	}

	// Check inputs of that tx
	highestDepth := uint(depth)
	missingMerkleProofAncestors := false
	for _, txin := range tx.Tx.TxIn {
		if etx.Ancestors.GetTx(txin.PreviousOutPoint.Hash) != nil {
			continue // tx already in ancestors
		}

		parentDepth, err := w.addAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash, depth+1)
		if err != nil {
			if errors.Cause(err) != MissingMerkleProofAncestors {
				return 0, errors.Wrap(err, txin.PreviousOutPoint.Hash.String())
			}
			missingMerkleProofAncestors = true
		}

		if parentDepth > highestDepth {
			highestDepth = parentDepth
		}
	}

	if missingMerkleProofAncestors {
		return highestDepth, MissingMerkleProofAncestors
	}

	return highestDepth, nil
}

// VerifyExpandedTx verifies the ancestors of an expanded tx and returns the depth of ancestry
// provided. It can also return MissingInputs or MissingMerkleProofAncestors which don't mean
// failure, but notify that full ancestry back to merkle proofs were not provided.
func (w *Wallet) VerifyExpandedTx(ctx context.Context, contextID bitcoin.Hash32,
	etx *channels.ExpandedTx) (uint, error) {

	verified := make(map[bitcoin.Hash32]bool)
	highestDepth := uint(0)
	missingInputs := false
	missingMerkleProofAncestors := false
	for _, txin := range etx.Tx.TxIn {
		depth, err := w.verifyAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash, 0,
			&verified)
		if err != nil {
			if errors.Cause(err) != MissingMerkleProofAncestors {
				return 0, errors.Wrap(err, txin.PreviousOutPoint.Hash.String())
			}

			if depth == 0 {
				missingInputs = true
			}

			missingMerkleProofAncestors = true
		}

		if depth > highestDepth {
			highestDepth = depth
		}
	}

	// Save txs for future use in expanded txs for descendents of this tx.
	for _, atx := range etx.Ancestors {
		tx := atx.GetTx()
		if tx == nil {
			continue
		}
		txid := *tx.TxHash()

		ftx, err := fetchTx(ctx, w.store, txid)
		if err != nil {
			return 0, errors.Wrapf(err, "fetch tx %s", txid)
		}
		if ftx != nil { // already have this tx
			modified := false

			if atx.MerkleProof != nil {
				if err := ftx.AddMerkleProof(ctx, atx.MerkleProof); err != nil {
					if errors.Cause(err) != AlreadyHaveMerkleProof {
						return 0, errors.Wrapf(err, "add merkle proof %s", txid)
					}
				} else {
					modified = true
				}
			}

			if ftx.AddContextID(contextID) {
				modified = true
			}

			if modified {
				if err := ftx.save(ctx, w.store); err != nil {
					return 0, errors.Wrapf(err, "save %s", txid)
				}
			}

			continue
		}

		// Create new tx
		fields := []logger.Field{
			logger.Stringer("txid", txid),
		}
		if atx.MerkleProof != nil {
			fields = append(fields, logger.Stringer("block_hash", atx.MerkleProof.GetBlockHash()))
		}
		logger.InfoWithFields(ctx, fields, "Saving expanded tx ancestor")

		ftx = &Tx{
			ContextIDs: []bitcoin.Hash32{contextID},
			Tx:         tx,
		}

		if atx.MerkleProof != nil {
			if err := ftx.AddMerkleProof(ctx, atx.MerkleProof); err != nil {
				if errors.Cause(err) != AlreadyHaveMerkleProof {
					return 0, errors.Wrap(err, "add merkle proof")
				}
			}
		}

		if err := ftx.save(ctx, w.store); err != nil {
			return 0, errors.Wrapf(err, "save tx %s", txid)
		}
	}

	if missingInputs {
		return highestDepth, MissingInput
	}

	if missingMerkleProofAncestors {
		return highestDepth, MissingMerkleProofAncestors
	}

	return highestDepth, nil
}

func (w *Wallet) verifyAncestorTx(ctx context.Context, etx *channels.ExpandedTx,
	txid bitcoin.Hash32, depth uint, verified *map[bitcoin.Hash32]bool) (uint, error) {

	if _, exists := (*verified)[txid]; exists {
		return 0, nil // already verified this tx
	}

	atx := etx.Ancestors.GetTx(txid)
	if atx == nil {
		return depth, errors.Wrap(MissingMerkleProofAncestors, txid.String())
	}

	if atx.MerkleProof != nil {
		_, isLongest, err := w.merkleProofVerifier.VerifyMerkleProof(ctx, atx.MerkleProof)
		if err != nil {
			return 0, errors.Wrapf(err, "merkle proof: %s", txid)
		}

		if isLongest {
			(*verified)[txid] = true
			return depth + 1, nil
		}

		return 0, errors.Wrap(ErrNotMostPOW, txid.String())
	}

	tx := atx.GetTx()
	if tx == nil {
		return depth, errors.Wrap(MissingMerkleProofAncestors, txid.String())
	}

	highestDepth := uint(0)
	missingMerkleProofAncestors := false
	for _, txin := range tx.TxIn {
		parentDepth, err := w.verifyAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash, depth+1,
			verified)
		if err != nil {
			if errors.Cause(err) != MissingMerkleProofAncestors {
				return 0, errors.Wrap(err, txid.String())
			}
			missingMerkleProofAncestors = true
		}

		if parentDepth > highestDepth {
			highestDepth = parentDepth
		}
	}

	(*verified)[txid] = true

	if missingMerkleProofAncestors {
		return highestDepth, MissingMerkleProofAncestors
	}
	return highestDepth, nil
}
