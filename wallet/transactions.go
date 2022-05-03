package wallet

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/storage"
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

	lock sync.RWMutex
}

func (w *Wallet) FetchTx(ctx context.Context, txid bitcoin.Hash32) (*Tx, error) {
	tx := &Tx{}
	if err := storage.Load(ctx, w.store, fmt.Sprintf("%s/%s", txsPath, txid), tx); err != nil {
		return nil, errors.Wrap(err, "storage")
	}

	return tx, nil
}

func (w *Wallet) SaveTx(ctx context.Context, tx *Tx) error {
	return storage.Save(ctx, w.store, fmt.Sprintf("%s/%s", txsPath, tx.Tx.TxHash()), tx)
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

	tx.MerkleProofs = append(tx.MerkleProofs, merkleProof)
	return nil
}

func (tx *Tx) Serialize(w io.Writer) error {
	tx.lock.RLock()
	scriptItems, err := bsor.Marshal(tx)
	if err != nil {
		tx.lock.RUnlock()
		return errors.Wrap(err, "bsor")
	}
	tx.lock.RUnlock()

	script, err := scriptItems.Script()
	if err != nil {
		return errors.Wrap(err, "script")
	}

	if err := binary.Write(w, endian, txsVersion); err != nil {
		return errors.Wrap(err, "version")
	}

	if _, err := w.Write(script); err != nil {
		return errors.Wrap(err, "write")
	}

	return nil
}

func (tx *Tx) Deserialize(r io.Reader) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "read")
	}

	var version uint8
	if err := binary.Read(r, endian, &version); err != nil {
		return errors.Wrap(err, "version")
	}
	if version != 0 {
		return errors.Wrap(ErrUnsupportedVersion, "tx")
	}

	scriptItems, err := bitcoin.ParseScriptItems(bytes.NewReader(b), -1)
	if err != nil {
		return errors.Wrap(err, "script")
	}

	tx.lock.Lock()
	if _, err := bsor.Unmarshal(scriptItems, tx); err != nil {
		tx.lock.Unlock()
		return errors.Wrap(err, "bsor")
	}
	tx.lock.Unlock()

	return nil
}
