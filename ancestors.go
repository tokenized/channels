package channels

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/json_envelope"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/wire"
)

// AncestorTx is a tx containing a spent output contained in an expanded tx or an ancestor. If it is
// confirmed then the merkle proof should be provided with the tx embedded in it, otherwise the
// tx with miner responses should be provided and the ancestors included in the same expanded tx.
type AncestorTx struct {
	MerkleProof    *merkle_proof.MerkleProof    `bsor:"1" json:"merkle_proof,omitempty"`
	Tx             *wire.MsgTx                  `bsor:"2" json:"tx,omitempty"`              // marshals as raw bytes
	MinerResponses []json_envelope.JSONEnvelope `bsor:"3" json:"miner_responses,omitempty"` // signed JSON envelope responses from miners for the tx
}

func (tx AncestorTx) GetTxID() *bitcoin.Hash32 {
	if tx.Tx != nil {
		return tx.Tx.TxHash()
	}

	if tx.MerkleProof != nil {
		return tx.MerkleProof.GetTxID()
	}

	return nil
}

func (tx AncestorTx) GetTx() *wire.MsgTx {
	if tx.Tx != nil {
		return tx.Tx
	}

	if tx.MerkleProof.Tx != nil {
		return tx.MerkleProof.Tx
	}

	return nil
}

func (tx AncestorTx) String() string {
	result := &bytes.Buffer{}
	if tx.MerkleProof != nil {
		result.Write([]byte(fmt.Sprintf("Merkle Proof : %s\n", tx.MerkleProof.String())))
	} else if tx.Tx != nil {
		result.Write([]byte(tx.Tx.String()))
	}

	result.Write([]byte(fmt.Sprintf("  %d Miner Responses\n", len(tx.MinerResponses))))
	for _, minerResponse := range tx.MinerResponses {
		js, _ := json.MarshalIndent(minerResponse, "    ", "  ")
		result.Write(append(js, []byte("\n")...))
	}

	return string(result.Bytes())
}

type AncestorTxs []*AncestorTx

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
