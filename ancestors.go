package channels

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/json_envelope"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

var (
	// MissingAncestor means ancestors don't go all the way to merkle proofs. This can reduce the
	// security of the ancestry, but may not always be a failure. It prevents knowing the
	// "unconfirmed depth" of the new tx, but doesn't mean it is invalid.
	MissingMerkleProofAncestors = errors.New("Missing Merkle Proof Ancestor")

	// MissingInput means that an ancestor spent by the main tx is missing. This doesn't
	// necessarily make it invalid, but is more serious than ErrMissingAncestor as many uses require
	// at least the immediate parents of the tx. For example fee calculation and script validation.
	MissingInput = errors.New("Missing Input")
)

// AncestorTx is a tx containing a spent output contained in an expanded tx or an ancestor. If it is
// confirmed then the merkle proof should be provided with the tx embedded in it, otherwise the
// tx with miner responses should be provided and the ancestors included in the same expanded tx.
type AncestorTx struct {
	Tx             *wire.MsgTx                  `bsor:"1" json:"tx,omitempty"` // marshals as raw bytes
	MerkleProofs   []*merkle_proof.MerkleProof  `bsor:"2" json:"merkle_proofs,omitempty"`
	MinerResponses []json_envelope.JSONEnvelope `bsor:"3" json:"miner_responses,omitempty"` // signed JSON envelope responses from miners for the tx
}

type AncestorTxs []*AncestorTx

func (tx AncestorTx) GetTxID() *bitcoin.Hash32 {
	if tx.Tx != nil {
		return tx.Tx.TxHash()
	}

	return nil
}

func (tx AncestorTx) GetTx() *wire.MsgTx {
	return tx.Tx
}

func (tx *AncestorTx) AddMerkleProof(merkleProof *merkle_proof.MerkleProof) bool {
	txid := merkleProof.GetTxID()
	if txid == nil {
		return false
	}

	blockHash := merkleProof.GetBlockHash()
	if blockHash == nil {
		return false
	}

	for _, mp := range tx.MerkleProofs {
		bh := mp.GetBlockHash()
		if bh == nil {
			continue
		}

		if bh.Equal(blockHash) {
			return false // already have this proof
		}
	}

	mp := *merkleProof
	if mp.Tx != nil {
		// Clear out tx so we don't have it duplicated
		mp.TxID = txid
		mp.Tx = nil
	}

	tx.MerkleProofs = append(tx.MerkleProofs, &mp)
	return true
}

func (tx AncestorTx) String() string {
	result := &bytes.Buffer{}

	if tx.Tx != nil {
		result.Write([]byte(tx.Tx.String()))
	}

	for _, mp := range tx.MerkleProofs {
		result.Write([]byte(fmt.Sprintf("Merkle Proof : %s\n", mp.String())))
	}

	result.Write([]byte(fmt.Sprintf("  %d Miner Responses\n", len(tx.MinerResponses))))
	for _, minerResponse := range tx.MinerResponses {
		js, _ := json.MarshalIndent(minerResponse, "    ", "  ")
		result.Write(append(js, []byte("\n")...))
	}

	return string(result.Bytes())
}

func (tx AncestorTx) StringWithAddresses(net bitcoin.Network) string {
	result := &bytes.Buffer{}

	if tx.Tx != nil {
		result.Write([]byte(tx.Tx.StringWithAddresses(net)))
	}

	for _, mp := range tx.MerkleProofs {
		result.Write([]byte(fmt.Sprintf("Merkle Proof : %s\n", mp.String())))
	}

	result.Write([]byte(fmt.Sprintf("  %d Miner Responses\n", len(tx.MinerResponses))))
	for _, minerResponse := range tx.MinerResponses {
		js, _ := json.MarshalIndent(minerResponse, "    ", "  ")
		result.Write(append(js, []byte("\n")...))
	}

	return string(result.Bytes())
}

func (txs AncestorTxs) GetTx(txid bitcoin.Hash32) *AncestorTx {
	for _, tx := range txs {
		ancestorTxID := tx.GetTxID()
		if ancestorTxID == nil {
			continue
		}

		if ancestorTxID.Equal(&txid) {
			return tx
		}
	}

	return nil
}

func (txs AncestorTxs) GetTxs() []*wire.MsgTx {
	result := make([]*wire.MsgTx, 0, len(txs))
	for _, atx := range txs {
		tx := atx.GetTx()
		if tx != nil {
			result = append(result, tx)
		}
	}

	return result
}

func (txs AncestorTxs) String() string {
	result := &bytes.Buffer{}
	result.Write([]byte(fmt.Sprintf("  %d Ancestors\n", len(txs))))
	for _, ancestor := range txs {
		result.Write([]byte(fmt.Sprintf("    %s\n", ancestor.String())))
	}

	return string(result.Bytes())
}

func (txs AncestorTxs) StringWithAddresses(net bitcoin.Network) string {
	result := &bytes.Buffer{}
	result.Write([]byte(fmt.Sprintf("  %d Ancestors\n", len(txs))))
	for _, ancestor := range txs {
		result.Write([]byte(fmt.Sprintf("    %s\n", ancestor.StringWithAddresses(net))))
	}

	return string(result.Bytes())
}
