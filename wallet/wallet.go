package wallet

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/logger"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/txbuilder"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

const (
	walletPath    = "channels_wallet"
	walletVersion = uint8(0)
)

var (
	ErrUnsupportedVersion = errors.New("Unsupported Version")
	ErrOutputAlreadySpent = errors.New("Output Already Spent")
	ErrUnknownTx          = errors.New("Unknown Tx")
	ErrMissingBlockHash   = errors.New("Missing Block Hash")
	ErrContextIDNotFound  = errors.New("Context ID Not Found")
	ErrUnknownHeader      = errors.New("Unkown Header")
	ErrMissingAncestor    = errors.New("Missing Ancestor")
	ErrNotMostPOW         = errors.New("Not Most POW")
	ErrContextNotFound    = errors.New("Context Not Found")
	ErrWrongKey           = errors.New("Wrong Key")

	AlreadyHaveMerkleProof = errors.New("Already Have Merkle Proof")

	endian = binary.LittleEndian
)

type Config struct {
	// SatoshiBreakValue is the lowest number used to split up satoshi values.
	SatoshiBreakValue uint64 `default:"10000" json:"satoshi_break_value"`

	// BreakCount is the most pieces a satoshi or token value will be broken into.
	BreakCount int `default:"5" json:"break_count"`
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

type Wallet struct {
	baseKey bitcoin.Key

	KeySet  KeySet
	Outputs Outputs

	config              Config
	store               storage.StreamReadWriter
	merkleProofVerifier MerkleProofVerifier
	feeQuoter           FeeQuoter

	lock sync.RWMutex
}

func NewWallet(config Config, store storage.StreamReadWriter,
	merkleProofVerifier MerkleProofVerifier, feeQuoter FeeQuoter, key bitcoin.Key) *Wallet {
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
	value uint64) (*channels.ExpandedTx, error) {

	feeQuotes, err := w.feeQuoter.GetFeeQuotes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fee quotes")
	}

	keys, err := w.GenerateKeys(contextID, w.config.BreakCount)
	if err != nil {
		return nil, errors.Wrap(err, "keys")
	}

	lockingScripts := make([]bitcoin.Script, len(keys))
	for i, key := range keys {
		lockingScripts[i] = key.LockingScript
	}

	outputs, err := txbuilder.BreakValueLockingScripts(value, w.config.SatoshiBreakValue,
		lockingScripts, feeQuotes.GetQuote(channels.FeeTypeStandard).RelayFee.Rate(),
		feeQuotes.GetQuote(channels.FeeTypeStandard).MiningFee.Rate(), false, false)
	if err != nil {
		return nil, errors.Wrap(err, "break value")
	}

	etx := &channels.ExpandedTx{
		Tx: wire.NewMsgTx(1),
	}

	for _, output := range outputs {
		etx.Tx.AddTxOut(&output.TxOut)
	}

	if err := w.PopulateExpandedTx(ctx, etx); err != nil {
		return nil, errors.Wrap(err, "populate")
	}

	return etx, nil
}

func (w *Wallet) FundTx(ctx context.Context, contextID bitcoin.Hash32,
	etx *channels.ExpandedTx) error {

	feeQuotes, err := w.feeQuoter.GetFeeQuotes(ctx)
	if err != nil {
		return errors.Wrap(err, "fee quotes")
	}

	miningFeeRate := feeQuotes.GetQuote(channels.FeeTypeStandard).MiningFee.Rate()
	relayFeeRate := feeQuotes.GetQuote(channels.FeeTypeStandard).RelayFee.Rate()

	txb, err := txbuilder.NewTxBuilderFromWire(miningFeeRate, relayFeeRate, etx.Tx,
		etx.Ancestors.GetTxs())

	utxos, err := w.SelectUTXOs(ctx, contextID, etx)
	if err != nil {
		return errors.Wrap(err, "utxos")
	}

	changeKeys, err := w.GenerateKeys(contextID, w.config.BreakCount)
	if err != nil {
		return errors.Wrap(err, "change keys")
	}

	var changeAddresses []txbuilder.AddressKeyID
	for i, changeKey := range changeKeys {
		ra, err := bitcoin.RawAddressFromLockingScript(changeKey.LockingScript)
		if err != nil {
			return errors.Wrapf(err, "change address %d", i)
		}

		changeAddresses = append(changeAddresses, txbuilder.AddressKeyID{
			Address: ra,
		})
	}

	if err := txb.AddFundingBreakChange(utxos, w.config.SatoshiBreakValue,
		changeAddresses); err != nil {
		return errors.Wrap(err, "funding")
	}

	if err := w.PopulateExpandedTx(ctx, etx); err != nil {
		return errors.Wrap(err, "populate")
	}

	return nil
}

func (w *Wallet) SignTx(ctx context.Context, contextID bitcoin.Hash32,
	etx *channels.ExpandedTx) error {

	wkeys, err := w.GetKeys(contextID)
	if err != nil {
		return errors.Wrap(err, "get keys")
	}

	keys := make([]bitcoin.Key, len(wkeys))
	for i, wkey := range wkeys {
		keys[i] = wkey.Key
	}

	feeQuotes, err := w.feeQuoter.GetFeeQuotes(ctx)
	if err != nil {
		return errors.Wrap(err, "fee quotes")
	}

	miningFeeRate := feeQuotes.GetQuote(channels.FeeTypeStandard).MiningFee.Rate()
	relayFeeRate := feeQuotes.GetQuote(channels.FeeTypeStandard).RelayFee.Rate()

	txb, err := txbuilder.NewTxBuilderFromWire(miningFeeRate, relayFeeRate, etx.Tx,
		etx.Ancestors.GetTxs())
	if err != nil {
		return errors.Wrap(err, "tx builder")
	}

	if err := txb.SignOnly(keys); err != nil {
		return errors.Wrap(err, "sign")
	}

	return nil
}

func (w *Wallet) SelectUTXOs(ctx context.Context, contextID bitcoin.Hash32,
	etx *channels.ExpandedTx) ([]bitcoin.UTXO, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	var result []bitcoin.UTXO
	for _, output := range w.Outputs {
		if output.SpentTxID != nil {
			continue
		}

		result = append(result, bitcoin.UTXO{
			Hash:          output.TxID,
			Index:         output.Index,
			Value:         output.Value,
			LockingScript: output.LockingScript,
		})
	}

	return result, nil
}

func (w *Wallet) fetchTx(ctx context.Context, txid bitcoin.Hash32) (*Tx, error) {
	tx := &Tx{}

	path := fmt.Sprintf("%s/%s", txsPath, txid)
	if err := storage.StreamRead(ctx, w.store, path, tx); err != nil {
		if errors.Cause(err) == storage.ErrNotFound {
			return nil, nil
		}
		return nil, errors.Wrap(err, "read")
	}

	return tx, nil
}

func (w *Wallet) saveTx(ctx context.Context, tx *Tx) error {
	path := fmt.Sprintf("%s/%s", txsPath, tx.Tx.TxHash())
	if err := storage.StreamWrite(ctx, w.store, path, tx); err != nil {
		return errors.Wrap(err, "write")
	}

	return nil
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

func (w *Wallet) Load(ctx context.Context) error {
	path := fmt.Sprintf("%s/wallet", walletPath)
	if err := storage.StreamRead(ctx, w.store, path, w); err != nil {
		return errors.Wrap(err, "read")
	}

	return nil
}

func (w *Wallet) Save(ctx context.Context) error {
	path := fmt.Sprintf("%s/wallet", walletPath)
	if err := storage.StreamWrite(ctx, w.store, path, w); err != nil {
		return errors.Wrap(err, "write")
	}

	return nil
}

func (w *Wallet) Serialize(writer io.Writer) error {
	w.lock.RLock()
	defer w.lock.RUnlock()

	if err := binary.Write(writer, endian, walletVersion); err != nil {
		return errors.Wrap(err, "version")
	}

	if err := w.baseKey.PublicKey().Serialize(writer); err != nil {
		return errors.Wrap(err, "public key")
	}

	if err := w.KeySet.Serialize(writer); err != nil {
		return errors.Wrap(err, "key set")
	}

	if err := w.Outputs.Serialize(writer); err != nil {
		return errors.Wrap(err, "outputs")
	}

	return nil
}

func (w *Wallet) Deserialize(r io.Reader) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	var version uint8
	if err := binary.Read(r, endian, &version); err != nil {
		return errors.Wrap(err, "version")
	}
	if version != 0 {
		return errors.New("Unsupported version")
	}

	var publicKey bitcoin.PublicKey
	if err := publicKey.Deserialize(r); err != nil {
		return errors.Wrap(err, "public key")
	}

	if !w.baseKey.PublicKey().Equal(publicKey) {
		return ErrWrongKey
	}

	if err := w.KeySet.Deserialize(r); err != nil {
		return errors.Wrap(err, "key set")
	}

	if err := w.Outputs.Deserialize(r); err != nil {
		return errors.Wrap(err, "outputs")
	}

	// Recalculate keys and locking scripts from hashes.
	for _, keys := range w.KeySet {
		for _, walletKey := range keys {
			key, err := w.baseKey.AddHash(walletKey.Hash)
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

	return nil
}
