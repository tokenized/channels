package wallet

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/txbuilder"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

const (
	walletPath = "channels_wallet"
)

var (
	ErrUnsupportedVersion = errors.New("Unsupported Version")
	ErrOutputAlreadySpent = errors.New("Output Already Spent")
	ErrUnknownTx          = errors.New("Unknown Tx")
	ErrMissingBlockHash   = errors.New("Missing Block Hash")
	ErrContextIDNotFound  = errors.New("Context ID Not Found")
	ErrUnknownHeader      = errors.New("Unkown Header")

	AlreadyHaveMerkleProof = errors.New("Already Have Merkle Proof")

	endian = binary.LittleEndian
)

type Wallet struct {
	baseKey       bitcoin.Key
	BasePublicKey bitcoin.PublicKey

	KeySet  KeySet
	Outputs Outputs

	config              Config
	store               storage.Storage
	merkleProofVerifier MerkleProofVerifier
	feeQuoter           FeeQuoter

	lock sync.RWMutex
}

type Config struct {
	FeeRate           float32 `json:"fee_rate"`
	DustFeeRate       float32 `json:"dust_fee_rate"`
	SatoshiBreakValue uint64  `json:"satoshi_break_value"`
	BreakCount        int     `json:"break_count"`
}

type MerkleProofVerifier interface {
	// VerifyMerkleProof finds the header in the block chain and verifies that the merkle proof is
	// properly linked to that block. It returns the block height and if it is in the longest chain.
	// It is possible that the merkle proof is valid, but linked to an orphaned block, or at least a
	// block that is not currently in the most proof of work chain. These merkle proofs should still
	// be retained as they may become part of the most proof of work chain later and at least show
	// that the tx was at one point accepted by a miner.
	VerifyMerkleProof(ctx context.Context, proof *merkle_proof.MerkleProof) (int, bool, error)
}

type FeeQuoter interface {
	// GetFeeQuotes retrieves an up to date fee quote to be used for applying a fee to a new tx.
	GetFeeQuotes(ctx context.Context) (channels.FeeQuotes, error)
}

func NewWallet(config Config, store storage.Storage, merkleProofVerifier MerkleProofVerifier,
	feeQuoter FeeQuoter, key bitcoin.Key) *Wallet {
	return &Wallet{
		config:              config,
		store:               store,
		merkleProofVerifier: merkleProofVerifier,
		feeQuoter:           feeQuoter,
		baseKey:             key,
		KeySet:              make(map[bitcoin.Hash32]Keys),
	}
}

// CreateBitcoinReceive creates a transaction receiving the specified amount of bitcoin.
func (w *Wallet) CreateBitcoinReceive(ctx context.Context, contextID bitcoin.Hash32,
	value uint64) (*wire.MsgTx, error) {

	keys, err := w.GenerateKeys(contextID, w.config.BreakCount)
	if err != nil {
		return nil, errors.Wrap(err, "keys")
	}

	lockingScripts := make([]bitcoin.Script, len(keys))
	for i, key := range keys {
		lockingScripts[i] = key.LockingScript
	}

	outputs, err := txbuilder.BreakValueLockingScripts(value, w.config.SatoshiBreakValue,
		lockingScripts, w.config.DustFeeRate, w.config.FeeRate, false, false)
	if err != nil {
		return nil, errors.Wrap(err, "break value")
	}

	tx := wire.NewMsgTx(1)
	for _, output := range outputs {
		tx.AddTxOut(&output.TxOut)
	}

	return tx, nil
}

func (w *Wallet) fetchTx(ctx context.Context, txid bitcoin.Hash32) (*Tx, error) {
	tx := &Tx{}
	if err := storage.Load(ctx, w.store, fmt.Sprintf("%s/%s", txsPath, txid), tx); err != nil {
		return nil, errors.Wrap(err, "storage")
	}

	return tx, nil
}

func (w *Wallet) saveTx(ctx context.Context, tx *Tx) error {
	return storage.Save(ctx, w.store, fmt.Sprintf("%s/%s", txsPath, tx.Tx.TxHash()), tx)
}

func (w *Wallet) AddTx(ctx context.Context, tx *wire.MsgTx, contextID bitcoin.Hash32) error {
	txid := *tx.TxHash()

	w.lock.Lock()
	defer w.lock.Unlock()

	keys, exists := w.KeySet[contextID]
	if !exists {
		return errors.Wrap(ErrContextIDNotFound, contextID.String())
	}

	// Check if tx was already added.
	walletTx, err := w.fetchTx(ctx, txid)
	if err != nil {
		return errors.Wrap(err, "fetch tx")
	}
	if walletTx != nil {
		return nil // already added tx
	}

	// Check for spent outputs.
	for _, output := range w.Outputs {
		for _, txin := range tx.TxIn {
			if !output.TxID.Equal(&txin.PreviousOutPoint.Hash) {
				continue
			}
			if output.Index != txin.PreviousOutPoint.Index {
				continue
			}

			if output.SpentTxID != nil {
				return errors.Wrap(ErrOutputAlreadySpent, output.SpentTxID.String())
			}

			output.SpentTxID = &txid
			logger.InfoWithFields(ctx, []logger.Field{
				logger.JSON("output", output),
			}, "Output spent")
		}
	}

	// Check for new outputs.
	for index, txout := range tx.TxOut {
		for _, key := range keys {
			if !txout.LockingScript.Equal(key.LockingScript) {
				continue
			}

			output := &Output{
				TxID:           txid,
				Index:          uint32(index),
				Value:          txout.Value,
				LockingScript:  txout.LockingScript,
				DerivationHash: &bitcoin.Hash32{},
			}
			copy(output.DerivationHash[:], key.Hash[:])

			logger.InfoWithFields(ctx, []logger.Field{
				logger.JSON("output", output),
			}, "New Output")
			w.Outputs = append(w.Outputs, output)
		}
	}

	walletTx = &Tx{
		Tx: tx,
	}
	if err := w.saveTx(ctx, walletTx); err != nil {
		return errors.Wrap(err, "save tx")
	}

	return nil
}

func (w *Wallet) AddMerkleProof(ctx context.Context, merkleProof *merkle_proof.MerkleProof) error {
	txid := merkleProof.GetTxID()
	if txid == nil {
		return errors.New("No txid in merkle proof")
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	// Check if tx was already added.
	walletTx, err := w.fetchTx(ctx, *txid)
	if err != nil {
		return errors.Wrap(err, "fetch tx")
	}
	if walletTx == nil {
		return errors.Wrap(ErrUnknownTx, txid.String())
	}

	if err := walletTx.AddMerkleProof(ctx, merkleProof); err != nil {
		if errors.Cause(err) == AlreadyHaveMerkleProof {
			return nil
		}
		return errors.Wrap(err, "add merkle proof")
	}

	if err := w.saveTx(ctx, walletTx); err != nil {
		return errors.Wrap(err, "save tx")
	}

	return nil
}

func (w *Wallet) BaseKey() bitcoin.Key {
	w.lock.RLock()
	result := w.baseKey
	w.lock.RUnlock()

	return result
}

func (w *Wallet) GetKeyForHash(hash bitcoin.Hash32) *Key {
	w.lock.RLock()
	defer w.lock.RUnlock()

	for _, keys := range w.KeySet {
		for _, key := range keys {
			if key.Hash.Equal(&hash) {
				return key
			}
		}
	}

	return nil
}

func (w *Wallet) GetKeyForLockingScript(script bitcoin.Script) *Key {
	w.lock.RLock()
	defer w.lock.RUnlock()

	for _, keys := range w.KeySet {
		for _, key := range keys {
			if key.LockingScript.Equal(script) {
				return key
			}
		}
	}

	return nil
}

// CheckKey is used to ensure the key matching the stored data was used to create the wallet. Call
// after loading the wallet from storage.
func (w *Wallet) CheckKey() error {
	w.lock.RLock()
	defer w.lock.RUnlock()

	if !w.baseKey.PublicKey().Equal(w.BasePublicKey) {
		return errors.New("Wrong key")
	}

	return nil
}

func (w *Wallet) Load(ctx context.Context) error {
	return storage.Load(ctx, w.store, fmt.Sprintf("%s/wallet", walletPath), w)
}

func (w *Wallet) Save(ctx context.Context) error {
	return storage.Save(ctx, w.store, fmt.Sprintf("%s/wallet", walletPath), w)
}

func (w *Wallet) Serialize(writer io.Writer) error {
	w.lock.RLock()
	scriptItems, err := bsor.Marshal(w)
	if err != nil {
		w.lock.RUnlock()
		return errors.Wrap(err, "bsor")
	}
	w.lock.RUnlock()

	script, err := scriptItems.Script()
	if err != nil {
		return errors.Wrap(err, "script")
	}

	if _, err := writer.Write(script); err != nil {
		return errors.Wrap(err, "write")
	}

	return nil
}

func (w *Wallet) Deserialize(r io.Reader) error {
	w.lock.RLock()
	baseKey := w.baseKey
	w.lock.RUnlock()

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "read")
	}

	scriptItems, err := bitcoin.ParseScriptItems(bytes.NewReader(b), -1)
	if err != nil {
		return errors.Wrap(err, "script")
	}

	w.lock.Lock()
	if _, err := bsor.Unmarshal(scriptItems, w); err != nil {
		w.lock.Unlock()
		return errors.Wrap(err, "bsor")
	}

	// Recalculate keys and locking scripts from hashes.
	for _, keys := range w.KeySet {
		for _, walletKey := range keys {
			key, err := baseKey.AddHash(walletKey.Hash)
			if err != nil {
				return errors.Wrap(err, "key")
			}
			walletKey.Key = key

			lockingScript, err := key.LockingScript()
			if err != nil {
				return errors.Wrap(err, "locking script")
			}
			walletKey.LockingScript = lockingScript
		}
	}

	w.lock.Unlock()

	return nil
}
