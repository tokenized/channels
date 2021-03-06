package channels

import (
	"bytes"
	"fmt"

	"github.com/tokenized/pkg/bitcoin"
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

func (etx ExpandedTx) StringWithAddresses(net bitcoin.Network) string {
	result := &bytes.Buffer{}
	if etx.Tx != nil {
		result.Write([]byte(fmt.Sprintf("%s\n", etx.Tx.StringWithAddresses(net))))
	}

	result.Write([]byte(etx.Ancestors.StringWithAddresses(net)))

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

func (etx ExpandedTx) InputCount() int {
	return len(etx.Tx.TxIn)
}

func (etx ExpandedTx) Input(index int) *wire.TxIn {
	return etx.Tx.TxIn[index]
}

func (etx ExpandedTx) InputLockingScript(index int) (bitcoin.Script, error) {
	if index >= len(etx.Tx.TxIn) {
		return nil, errors.New("Index out of range")
	}

	txin := etx.Tx.TxIn[index]

	parentTx := etx.Ancestors.GetTx(txin.PreviousOutPoint.Hash)
	if parentTx == nil {
		return nil, errors.Wrap(MissingInput, "parent:"+txin.PreviousOutPoint.Hash.String())
	}

	tx := parentTx.GetTx()
	if tx == nil {
		return nil, errors.Wrap(MissingInput, "parent tx:"+txin.PreviousOutPoint.Hash.String())
	}

	if txin.PreviousOutPoint.Index >= uint32(len(tx.TxOut)) {
		return nil, errors.Wrap(MissingInput, txin.PreviousOutPoint.String())
	}

	return tx.TxOut[txin.PreviousOutPoint.Index].LockingScript, nil
}

func (etx ExpandedTx) OutputCount() int {
	return len(etx.Tx.TxOut)
}

func (etx ExpandedTx) Output(index int) *wire.TxOut {
	return etx.Tx.TxOut[index]
}
