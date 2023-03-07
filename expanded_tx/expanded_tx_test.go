package expanded_tx

import (
	"bytes"
	"encoding/hex"
	"io"
	"os"
	"testing"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/expanded_tx"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

func Test_Decode(t *testing.T) {
	file, err := os.Open("sample.etx.hex")
	if err != nil {
		t.Fatalf("Failed to open file : %s", err)
	}
	defer file.Close()

	fileSize, err := file.Seek(0, os.SEEK_END)
	if err != nil {
		t.Fatalf("Failed to seek end : %s", err)
	}
	if fileSize%2 != 0 {
		fileSize-- // remove new line at end
	}
	t.Logf("File size : %d", fileSize)

	if _, err := file.Seek(0, os.SEEK_SET); err != nil {
		t.Fatalf("Failed to seek begin : %s", err)
	}

	h := make([]byte, fileSize)
	if readSize, err := file.Read(h); err != nil {
		t.Fatalf("Failed to read file : %s", err)
	} else if readSize != int(fileSize) {
		t.Fatalf("Failed to read full file: read %d, size %d", readSize, fileSize)
	}

	// t.Logf("Hex : %s", string(h))

	b, err := hex.DecodeString(string(h))
	if err != nil {
		t.Fatalf("Failed decode hex at offset %d : %s", len(b), err)
	}
	script := bitcoin.Script(b)

	protocols := channels.NewProtocols(NewProtocol(),
		channels.NewReplyToProtocol())

	msg, _, err := protocols.Parse(script)
	if err != nil {
		t.Fatalf("Failed to parse script : %s", err)
	}

	cetx, ok := msg.(*ExpandedTxMessage)
	if !ok {
		t.Fatalf("Not a channels expanded tx")
	}

	etx := expanded_tx.ExpandedTx(*cetx)
	// t.Logf("Expanded Tx : %s", etx)

	setx := &SendExpandedTx{
		Tx:      &etx,
		Indexes: []uint32{1, 2, 3},
	}

	buf := &bytes.Buffer{}
	if err := setx.Serialize(buf); err != nil {
		t.Fatalf("Failed to serialize send etx : %s", err)
	}

	nsetx := &SendExpandedTx{}
	if err := nsetx.Deserialize(buf); err != nil {
		t.Fatalf("Failed to deserialize send etx : %s", err)
	}

	t.Logf("Send Expanded TxID : %s", setx.Tx.TxID())
	t.Logf("Indexes : %v", setx.Indexes)
}

type SendExpandedTx struct {
	Tx      *expanded_tx.ExpandedTx
	Indexes []uint32
}

// Deserialize reads the message from a reader.
func (m *SendExpandedTx) Deserialize(r io.Reader) error {
	txSize, err := wire.ReadVarInt(r, wire.ProtocolVersion)
	if err != nil {
		return errors.Wrap(err, "tx size")
	}

	script := make(bitcoin.Script, txSize)
	if _, err := r.Read(script); err != nil {
		return errors.Wrap(err, "script")
	}

	m.Tx = &expanded_tx.ExpandedTx{}
	if _, err := bsor.UnmarshalBinary(script, m.Tx); err != nil {
		return errors.Wrap(err, "tx")
	}

	count, err := wire.ReadVarInt(r, wire.ProtocolVersion)
	if err != nil {
		return errors.Wrap(err, "count")
	}

	m.Indexes = make([]uint32, count)
	for i := range m.Indexes {
		index, err := wire.ReadVarInt(r, wire.ProtocolVersion)
		if err != nil {
			return errors.Wrapf(err, "index %d", i)
		}
		m.Indexes[i] = uint32(index)
	}

	return nil
}

// Serialize writes the message to a writer.
func (m SendExpandedTx) Serialize(w io.Writer) error {
	script, err := bsor.MarshalBinary(m.Tx)
	if err != nil {
		return errors.Wrap(err, "tx")
	}

	if err := wire.WriteVarInt(w, wire.ProtocolVersion, uint64(len(script))); err != nil {
		return errors.Wrap(err, "tx size")
	}

	if _, err := w.Write(script); err != nil {
		return errors.Wrap(err, "script")
	}

	if err := wire.WriteVarInt(w, wire.ProtocolVersion, uint64(len(m.Indexes))); err != nil {
		return errors.Wrap(err, "count")
	}

	for i, index := range m.Indexes {
		if err := wire.WriteVarInt(w, wire.ProtocolVersion, uint64(index)); err != nil {
			return errors.Wrapf(err, "index %d", i)
		}
	}

	return nil
}
