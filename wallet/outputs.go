package wallet

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/storage"

	"github.com/pkg/errors"
)

const (
	outputsPath        = "channels_wallet/outputs"
	archiveOutputsPath = "channels_wallet/archived_outputs"
	outputsVersion     = uint8(0)
)

type Output struct {
	TxID              bitcoin.Hash32     `bsor:"1" json:"txid"`
	Index             uint32             `bsor:"2" json:"index"`
	Value             uint64             `bsor:"3" json:"value"`
	State             TxState            `bsor:"4" json:"safe,omitempty"`
	LockingScript     bitcoin.Script     `bsor:"5" json:"locking_script"`
	DerivationHash    *bitcoin.Hash32    `bsor:"6" json:"derivation_hash`
	SpentTxID         *bitcoin.Hash32    `bsor:"7" json:"spent_txid"`
	SpentHeight       uint32             `bsor:"8" json:"spent_height"`
	ReservedContextID *bitcoin.Hash32    `bsor:"9" json:"reserved_context_id"`
	Timestamp         channels.Timestamp `bsor:"10" json:"timestamp"`

	modified bool // when true this output needs to be written to storage
}

type Outputs []*Output

type OutputsByTimestamp Outputs

func (os OutputsByTimestamp) Len() int {
	return len(os)
}

func (os OutputsByTimestamp) Swap(i, j int) {
	os[i], os[j] = os[j], os[i]
}

func (os OutputsByTimestamp) Less(i, j int) bool {
	if os[i].Timestamp == os[j].Timestamp {
		return os[i].TxID[0] < os[j].TxID[0]
	}
	return os[i].Timestamp < os[j].Timestamp
}

func (os *Outputs) load(ctx context.Context, store storage.StreamStorage) error {
	paths, err := store.List(ctx, outputsPath)
	if err != nil {
		return errors.Wrap(err, "list")
	}

	for _, path := range paths {
		var outputs Outputs
		if err := storage.StreamRead(ctx, store, path, &outputs); err != nil {
			return errors.Wrapf(err, "read %s", path)
		}

		*os = append(*os, outputs...)
	}

	sort.Sort(OutputsByTimestamp(*os))
	return nil
}

func (os *Outputs) save(ctx context.Context, store storage.StreamStorage,
	blockHeight uint32) error {

	paths, err := store.List(ctx, outputsPath)
	if err != nil {
		return errors.Wrap(err, "list")
	}

	sort.Sort(OutputsByTimestamp(*os))
	parts := os.split()
	pruneHeight := uint32(0)
	if blockHeight > pruneDepth {
		pruneHeight = blockHeight - pruneDepth
	}
	var newOutputs Outputs

	for lookup, outputs := range parts {
		// Archive any outputs spent below blockHeight - pruneDepth.
		modified := false
		var archiveOutputs, unarchivedOutputs Outputs
		for _, output := range outputs {
			if output.SpentHeight != 0 && output.SpentHeight < pruneHeight {
				archiveOutputs = append(archiveOutputs, output)
				modified = true
				continue
			}

			if output.modified {
				modified = true
			}
			unarchivedOutputs = append(unarchivedOutputs, output)
		}

		path := fmt.Sprintf("%s/0x%02x", outputsPath, lookup)

		if !modified {
			if len(unarchivedOutputs) > 0 {
				paths = removePath(paths, path)
				newOutputs = append(newOutputs, unarchivedOutputs...)
			}
			continue
		}

		if len(unarchivedOutputs) > 0 {
			if err := storage.StreamWrite(ctx, store, path, &unarchivedOutputs); err != nil {
				return errors.Wrapf(err, "write %s", path)
			}

			for _, output := range unarchivedOutputs {
				output.modified = false
			}

			paths = removePath(paths, path)
			newOutputs = append(newOutputs, unarchivedOutputs...)
		}

		if len(archiveOutputs) > 0 {
			archivePath := fmt.Sprintf("%s/0x%02x", archiveOutputsPath, lookup)

			// Read previously archived outputs.
			var archivedOutputs Outputs
			if err := storage.StreamRead(ctx, store, archivePath, &archivedOutputs); err != nil {
				if errors.Cause(err) == storage.ErrNotFound {
					archivedOutputs = nil // make sure it is empty after error
				} else {
					return errors.Wrapf(err, "read %s", archivePath)
				}
			}

			// Append new archived outputs.
			archivedOutputs = append(archivedOutputs, archiveOutputs...)

			if err := storage.StreamWrite(ctx, store, archivePath, &archivedOutputs); err != nil {
				return errors.Wrapf(err, "write %s", archivePath)
			}
		}
	}

	// Remove any parts that no longer exist.
	for _, path := range paths {
		if err := store.Remove(ctx, path); err != nil {
			return errors.Wrapf(err, "remove %s", path)
		}
	}

	sort.Sort(OutputsByTimestamp(newOutputs))
	*os = newOutputs
	return nil
}

func removePath(paths []string, path string) []string {
	for i, p := range paths {
		if p == path {
			return append(paths[:i], paths[i+1:]...)
		}
	}

	return paths
}

func (os Outputs) split() map[byte]Outputs {
	result := make(map[byte]Outputs)
	for _, output := range os {
		lookup := output.TxID[0]
		set, exists := result[lookup]
		if exists {
			result[lookup] = append(set, output)
		} else {
			result[lookup] = Outputs{output}
		}
	}

	return result
}

func (o Output) Serialize(w io.Writer) error {
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

func (os Outputs) Serialize(w io.Writer) error {
	if err := binary.Write(w, endian, outputsVersion); err != nil {
		return errors.Wrap(err, "version")
	}

	if err := binary.Write(w, endian, uint32(len(os))); err != nil {
		return errors.Wrap(err, "count")
	}

	for i, o := range os {
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
