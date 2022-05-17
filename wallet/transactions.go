package wallet

import (
	"context"
	"encoding/binary"
	"io"
	"io/ioutil"

	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

const (
	txsPath    = "channels_wallet/txs"
	txsVersion = uint8(0)
)

type Tx struct {
	Tx           *wire.MsgTx                 `bsor:"1" json:"tx"`
	MerkleProofs []*merkle_proof.MerkleProof `bsor:"2" json:"merkle_proofs,omitempty"`
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
