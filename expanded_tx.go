package channels

import (
	"bytes"
	"fmt"

	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

var (
	// ErrNegativeFee means the tx has more output value than input value.
	ErrNegativeFee = errors.New("Negative Fee")
)

// ExpandedTx is a Bitcoin transaction with ancestor information.
// All ancestor transactions back to merkle proofs should be provided.
type ExpandedTx struct {
	Tx        *wire.MsgTx `bsor:"1" json:"tx"`                  // marshals as raw bytes
	Ancestors AncestorTxs `bsor:"2" json:"ancestors,omitempty"` // ancestor history of outputs up to merkle proofs
}

func (etx ExpandedTx) String() string {
	result := &bytes.Buffer{}
	if etx.Tx != nil {
		result.Write([]byte(fmt.Sprintf("%s\n", etx.Tx.String())))
	}

	result.Write([]byte(etx.Ancestors.String()))

	return string(result.Bytes())
}

// CalculateFee calculates the mining fee paid by the tx.
// Note: If transactions contianing outputs spent by the inputs are not included in ancestors then
// `MissingInput` will be returned.
func (etx ExpandedTx) CalculateFee() (uint64, error) {
	inputValue := uint64(0)
	for _, txin := range etx.Tx.TxIn {
		parentTx := etx.Ancestors.GetTx(txin.PreviousOutPoint.Hash)
		if parentTx == nil {
			return 0, errors.Wrap(MissingInput, "parent:"+txin.PreviousOutPoint.Hash.String())
		}

		tx := parentTx.GetTx()
		if tx == nil {
			return 0, errors.Wrap(MissingInput, "parent tx:"+txin.PreviousOutPoint.Hash.String())
		}

		if txin.PreviousOutPoint.Index >= uint32(len(tx.TxOut)) {
			return 0, errors.Wrap(MissingInput, txin.PreviousOutPoint.String())
		}

		inputValue += tx.TxOut[txin.PreviousOutPoint.Index].Value
	}

	outputValue := uint64(0)
	for _, txout := range etx.Tx.TxOut {
		outputValue += txout.Value
	}

	if outputValue > inputValue {
		return 0, ErrNegativeFee
	}

	return inputValue - outputValue, nil
}
