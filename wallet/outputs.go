package wallet

import (
	"encoding/binary"
	"io"

	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"

	"github.com/pkg/errors"
)

const (
	outputsPath    = "channels_wallet/outputs"
	outputsVersion = uint8(0)
)

type Output struct {
	TxID              bitcoin.Hash32  `bsor:"1" json:"txid"`
	Index             uint32          `bsor:"2" json:"index"`
	Value             uint64          `bsor:"3" json:"value"`
	LockingScript     bitcoin.Script  `bsor:"4" json:"locking_script"`
	DerivationHash    *bitcoin.Hash32 `bsor:"5" json:"derivation_hash`
	SpentTxID         *bitcoin.Hash32 `bsor:"6" json:"spent_txid"`
	ReservedContextID *bitcoin.Hash32 `bsor:"7" json:"reserved_context_id"`
}

type Outputs []*Output

func (o *Output) Serialize(w io.Writer) error {
	b, err := bsor.MarshalBinary(o)
	if err != nil {
		return errors.Wrap(err, "marshal")
	}

	if err := binary.Write(w, endian, uint32(len(b))); err != nil {
		return errors.Wrap(err, "size")
	}

	if _, err := w.Write(b); err != nil {
		return errors.Wrap(err, "bytes")
	}

	return nil
}

func (o *Output) Deserialize(r io.Reader) error {
	var size uint32
	if err := binary.Read(r, endian, &size); err != nil {
		return errors.Wrap(err, "size")
	}

	b := make([]byte, size)
	if _, err := io.ReadFull(r, b); err != nil {
		return errors.Wrap(err, "bytes")
	}

	if _, err := bsor.UnmarshalBinary(b, o); err != nil {
		return errors.Wrap(err, "unmarshal")
	}

	return nil
}

func (os *Outputs) Serialize(w io.Writer) error {
	if err := binary.Write(w, endian, outputsVersion); err != nil {
		return errors.Wrap(err, "version")
	}

	if err := binary.Write(w, endian, uint32(len(*os))); err != nil {
		return errors.Wrap(err, "count")
	}

	for i, o := range *os {
		if err := o.Serialize(w); err != nil {
			return errors.Wrapf(err, "output %d", i)
		}
	}

	return nil
}

func (os *Outputs) Deserialize(r io.Reader) error {
	var version uint8
	if err := binary.Read(r, endian, &version); err != nil {
		return errors.Wrap(err, "version")
	}
	if version != 0 {
		return errors.New("Unsupported version")
	}

	var count uint32
	if err := binary.Read(r, endian, &count); err != nil {
		return errors.Wrap(err, "count")
	}

	result := make(Outputs, count)
	for i := range result {
		newOutput := &Output{}
		if err := newOutput.Deserialize(r); err != nil {
			return errors.Wrapf(err, "output %d", i)
		}

		result[i] = newOutput
	}

	*os = result
	return nil
}
