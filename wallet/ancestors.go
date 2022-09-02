package wallet

import (
	"context"
	"fmt"

	"github.com/tokenized/channels"
	"github.com/tokenized/logger"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/expanded_tx"

	"github.com/pkg/errors"
)

const (
	minimumFeeOverpaid = uint64(500)
)

var (
	// InsufficientFee means the tx doesn't pay enough fee.
	InsufficientFee = errors.New("Insufficient Fee")

	// FeeOverpaid means the fee is well over the required fee and is probably a fee calculation
	// error.
	FeeOverpaid = errors.New("Fee Overpaid")
)

// VerifyFee verifies that the tx meets the specified fee requirements.
// Note: If VerifyExpandedTx returned `MissingInput` then this function will fail as the input
// values are not available.
func (w *Wallet) VerifyFee(ctx context.Context, contextID bitcoin.Hash32,
	etx *expanded_tx.ExpandedTx, requirements channels.FeeRequirements) error {

	fee, err := etx.CalculateFee()
	if err != nil {
		return errors.Wrap(err, "fee")
	}

	feeByteCounts := channels.TxFeeByteCounts(etx.Tx)
	if err != nil {
		return errors.Wrap(err, "fee byte counts")
	}

	requiredFee := requirements.RequiredFee(feeByteCounts)
	if fee < (requiredFee*98)/100 { // less than 98% of required fee
		return errors.Wrapf(InsufficientFee, "%d < %d", fee, requiredFee)
	}

	standardRequirement := requirements.GetStandardRequirement()
	standardRequiredFee := (uint64(etx.Tx.SerializeSize()) * standardRequirement.Satoshis) /
		standardRequirement.Bytes
	if standardRequiredFee < minimumFeeOverpaid {
		standardRequiredFee = minimumFeeOverpaid
	}

	// Detect if fee is way higher and probably in error.
	if fee > 5*standardRequiredFee {
		return errors.Wrapf(FeeOverpaid, "%d > %d", fee, requiredFee)
	}

	return nil
}

// PopulateExpandedTx adds any ancestors that are needed for the tx. This should be called after
// funding the tx. It returns the depth of ancestry provided. It can also return MissingInputs or
// MissingMerkleProofAncestors which don't mean failure, but notify that full ancestry back to
// merkle proofs were not available.
func (w *Wallet) PopulateExpandedTx(ctx context.Context, etx *expanded_tx.ExpandedTx) (uint, error) {
	highestDepth := uint(0)
	missingInputs := false
	missingMerkleProofAncestors := false
	for _, txin := range etx.Tx.TxIn {
		if etx.Ancestors.GetTx(txin.PreviousOutPoint.Hash) != nil {
			continue // tx already in ancestors
		}

		depth, err := w.addAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash, 0)
		if err != nil {
			if errors.Cause(err) != expanded_tx.MissingMerkleProofAncestors {
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
		return highestDepth, expanded_tx.MissingInput
	}

	if missingMerkleProofAncestors {
		return highestDepth, expanded_tx.MissingMerkleProofAncestors
	}

	return highestDepth, nil
}

func (w *Wallet) addAncestorTx(ctx context.Context, etx *expanded_tx.ExpandedTx,
	txid bitcoin.Hash32, depth uint) (uint, error) {

	tx, err := fetchTx(ctx, w.store, txid)
	if err != nil {
		return 0, errors.Wrap(err, "fetch tx")
	}
	if tx == nil {
		return depth, expanded_tx.MissingMerkleProofAncestors
	}

	ancestor := &expanded_tx.AncestorTx{
		Tx: tx.Tx,
	}
	etx.Ancestors = append(etx.Ancestors, ancestor)

	proof, err := tx.GetMerkleProof(ctx, w.merkleProofVerifier)
	if err != nil {
		return 0, errors.Wrap(err, "tx merkle proof")
	}
	if proof != nil {
		ancestor.AddMerkleProof(proof)
		return depth + 1, nil
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
			if errors.Cause(err) != expanded_tx.MissingMerkleProofAncestors {
				return 0, errors.Wrap(err, txin.PreviousOutPoint.Hash.String())
			}
			missingMerkleProofAncestors = true
		}

		if parentDepth > highestDepth {
			highestDepth = parentDepth
		}
	}

	if missingMerkleProofAncestors {
		return highestDepth, expanded_tx.MissingMerkleProofAncestors
	}

	return highestDepth, nil
}

// VerifyExpandedTx verifies the ancestors of an expanded tx and returns the depth of ancestry
// provided. It can also return MissingInputs or MissingMerkleProofAncestors which don't mean
// failure, but notify that full ancestry back to merkle proofs were not provided.
func (w *Wallet) VerifyExpandedTx(ctx context.Context, contextID bitcoin.Hash32,
	etx *expanded_tx.ExpandedTx) (uint, error) {

	verified := make(map[bitcoin.Hash32]bool)
	highestDepth := uint(0)
	missingInputs := false
	missingMerkleProofAncestors := false
	for _, txin := range etx.Tx.TxIn {
		depth, err := w.verifyAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash, 0,
			&verified)
		if err != nil {
			if errors.Cause(err) != expanded_tx.MissingMerkleProofAncestors {
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

			for _, mp := range atx.MerkleProofs {
				if err := ftx.AddMerkleProof(ctx, mp); err != nil {
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
			logger.Stringer("ancestor_txid", txid),
		}
		if len(atx.MerkleProofs) > 0 {
			blockHashes := make([]fmt.Stringer, len(atx.MerkleProofs))
			for i, mp := range atx.MerkleProofs {
				blockHashes[i] = mp.GetBlockHash()
			}
			fields = append(fields, logger.Stringers("block_hashes", blockHashes))
		}
		logger.InfoWithFields(ctx, fields, "Saving expanded tx ancestor")

		ftx = &Tx{
			ContextIDs: []bitcoin.Hash32{contextID},
			Tx:         tx,
		}

		for _, mp := range atx.MerkleProofs {
			if err := ftx.AddMerkleProof(ctx, mp); err != nil {
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
		return highestDepth, expanded_tx.MissingInput
	}

	if missingMerkleProofAncestors {
		return highestDepth, expanded_tx.MissingMerkleProofAncestors
	}

	return highestDepth, nil
}

func (w *Wallet) verifyAncestorTx(ctx context.Context, etx *expanded_tx.ExpandedTx,
	txid bitcoin.Hash32, depth uint, verified *map[bitcoin.Hash32]bool) (uint, error) {

	if _, exists := (*verified)[txid]; exists {
		return 0, nil // already verified this tx
	}

	atx := etx.Ancestors.GetTx(txid)
	if atx == nil {
		return depth, errors.Wrap(expanded_tx.MissingMerkleProofAncestors, txid.String())
	}

	if len(atx.MerkleProofs) > 0 {
		containsLongest := false
		for _, mp := range atx.MerkleProofs {
			_, isLongest, err := w.merkleProofVerifier.VerifyMerkleProof(ctx, mp)
			if err != nil {
				continue
			}

			if isLongest {
				(*verified)[txid] = true
				return depth + 1, nil
			}
		}

		if !containsLongest {
			return 0, errors.Wrap(ErrNotMostPOW, txid.String())
		}
	}

	tx := atx.GetTx()
	if tx == nil {
		return depth, errors.Wrap(expanded_tx.MissingMerkleProofAncestors, txid.String())
	}

	highestDepth := uint(0)
	missingMerkleProofAncestors := false
	for _, txin := range tx.TxIn {
		parentDepth, err := w.verifyAncestorTx(ctx, etx, txin.PreviousOutPoint.Hash, depth+1,
			verified)
		if err != nil {
			if errors.Cause(err) != expanded_tx.MissingMerkleProofAncestors {
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
		return highestDepth, expanded_tx.MissingMerkleProofAncestors
	}
	return highestDepth, nil
}
