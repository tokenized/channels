package wallet

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/tokenized/logger"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

const (
	TxStatePending   = TxState(0) // Initial time period to see double spend attempts has not passed.
	TxStateSafe      = TxState(1) // No conflicting txs seen for initial time period.
	TxStateUnsafe    = TxState(2) // Conflicting tx seen, but not confirmed.
	TxStateCancelled = TxState(4) // Conflicting tx confirmed.

	txsPath    = "channels_wallet/txs"
	txsVersion = uint8(0)
)

type TxState uint8

type Tx struct {
	// ContextIDs are used to link txs to different external contexts. For example linking a tx to a
	// relationship. Multiples are needed since a tx could be received from one tx and spent to
	// another, or spent to more than one when it is unconfirmed, which would require sending the
	// merkle proof to both relationships when it confirms.
	ContextIDs   []bitcoin.Hash32            `bsor:"1" json:"context_ids"`
	Tx           *wire.MsgTx                 `bsor:"2" json:"tx"`
	State        TxState                     `bsor:"3" json:"safe,omitempty"`
	MerkleProofs []*merkle_proof.MerkleProof `bsor:"4" json:"merkle_proofs,omitempty"`
}

func (tx Tx) GetMerkleProof(ctx context.Context,
	verifier MerkleProofVerifier) (*merkle_proof.MerkleProof, error) {

	for _, proof := range tx.MerkleProofs {
		_, isLongest, err := verifier.VerifyMerkleProof(ctx, proof)
		if err != nil {
			return nil, errors.Wrap(err, "verify")
		}

		if isLongest {
			return proof, nil
		}
	}

	return nil, nil
}

// AddContextID adds a context id to a tx and returns true if it wasn't already there.
func (tx *Tx) AddContextID(hash bitcoin.Hash32) bool {
	for _, contextID := range tx.ContextIDs {
		if contextID.Equal(&hash) {
			return false
		}
	}

	tx.ContextIDs = append(tx.ContextIDs, hash)
	return true
}

func (tx *Tx) AddMerkleProof(ctx context.Context, merkleProof *merkle_proof.MerkleProof) error {
	blockHash := merkleProof.GetBlockHash()
	if blockHash == nil {
		return ErrMissingBlockHash
	}

	for _, mp := range tx.MerkleProofs {
		itemBlockHash := mp.GetBlockHash()
		if itemBlockHash == nil {
			continue
		}

		if itemBlockHash.Equal(blockHash) {
			return AlreadyHaveMerkleProof
		}
	}

	if merkleProof.Tx != nil {
		merkleProof.TxID = merkleProof.Tx.TxHash()
		merkleProof.Tx = nil
	}

	tx.MerkleProofs = append(tx.MerkleProofs, merkleProof)

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("txid", tx.Tx.TxHash()),
		logger.Stringer("block_hash", blockHash),
	}, "Added merkle proof to tx")
	return nil
}

func fetchTx(ctx context.Context, store storage.StreamReadWriter,
	txid bitcoin.Hash32) (*Tx, error) {

	tx := &Tx{}

	path := fmt.Sprintf("%s/%s", txsPath, txid)
	if err := storage.StreamRead(ctx, store, path, tx); err != nil {
		if errors.Cause(err) == storage.ErrNotFound {
			return nil, nil
		}
		return nil, errors.Wrap(err, "read")
	}

	return tx, nil
}

func (tx *Tx) save(ctx context.Context, store storage.StreamReadWriter) error {
	path := fmt.Sprintf("%s/%s", txsPath, tx.Tx.TxHash())
	if err := storage.StreamWrite(ctx, store, path, tx); err != nil {
		return errors.Wrap(err, "write")
	}

	return nil
}

func (tx Tx) Serialize(w io.Writer) error {
	b, err := bsor.MarshalBinary(tx)
	if err != nil {
		return errors.Wrap(err, "bsor")
	}

	if err := binary.Write(w, endian, txsVersion); err != nil {
		return errors.Wrap(err, "version")
	}

	if _, err := w.Write(b); err != nil {
		return errors.Wrap(err, "write")
	}

	return nil
}

func (tx *Tx) Deserialize(r io.Reader) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "read")
	}

	if b[0] != 0 { // version
		return errors.Wrap(ErrUnsupportedVersion, "tx")
	}

	if _, err := bsor.UnmarshalBinary(b[1:], tx); err != nil {
		return errors.Wrap(err, "bsor")
	}

	return nil
}

func (v *TxState) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for TxState : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v TxState) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v TxState) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown TxState value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *TxState) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *TxState) SetString(s string) error {
	switch s {
	case "pending":
		*v = TxStatePending
	case "safe":
		*v = TxStateSafe
	case "unsafe":
		*v = TxStateUnsafe
	case "cancelled":
		*v = TxStateCancelled
	default:
		*v = TxStatePending
		return fmt.Errorf("Unknown TxState value \"%s\"", s)
	}

	return nil
}

func (v TxState) String() string {
	switch v {
	case TxStatePending:
		return "pending"
	case TxStateSafe:
		return "safe"
	case TxStateUnsafe:
		return "unsafe"
	case TxStateCancelled:
		return "cancelled"
	default:
		return ""
	}
}
