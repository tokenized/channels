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
	"github.com/tokenized/pkg/merchant_api"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/txbuilder"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

const (
	walletPath    = "channels_wallet"
	walletVersion = uint8(0)

	// pruneDepth is the number of blocks below the latest block height at which spent UTXOs and
	// confirmed and spent transactions will be moved to "archive" storage.
	pruneDepth = uint32(1000)
)

var (
	ErrUnsupportedVersion = errors.New("Unsupported Version")
	ErrOutputAlreadySpent = errors.New("Output Already Spent")
	ErrUnknownTx          = errors.New("Unknown Tx")
	ErrMissingBlockHash   = errors.New("Missing Block Hash")
	ErrContextIDNotFound  = errors.New("Context ID Not Found")
	ErrUnknownHeader      = errors.New("Unkown Header")
	ErrNotMostPOW         = errors.New("Not Most POW")
	ErrContextNotFound    = errors.New("Context Not Found")
	ErrWrongKey           = errors.New("Wrong Key")
	ErrMissingOutput      = errors.New("Missing Output")

	AlreadyHaveMerkleProof = errors.New("Already Have Merkle Proof")

	endian = binary.LittleEndian
)

type Config struct {
	// SatoshiBreakValue is the lowest number used to split up satoshi values.
	SatoshiBreakValue uint64 `default:"10000" json:"satoshi_break_value" envconfig:"SATOSHI_BREAK_VALUE"`

	// BreakCount is the most pieces a satoshi or token value will be broken into.
	BreakCount int `default:"5" json:"break_count" envconfig:"BREAK_COUNT"`
}

func DefaultConfig() Config {
	return Config{
		SatoshiBreakValue: 10000,
		BreakCount:        5,
	}
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
	GetFeeQuotes(ctx context.Context) (merchant_api.FeeQuotes, error)
}

type Wallet struct {
	baseKey bitcoin.Key

	txLock sync.Mutex

	keys     KeySet
	keysLock sync.RWMutex

	outputs     Outputs
	outputsLock sync.RWMutex

	blockHeight uint32

	config              Config
	store               storage.StreamStorage
	merkleProofVerifier MerkleProofVerifier
	feeQuoter           FeeQuoter

	lock sync.RWMutex
}

func NewWallet(config Config, store storage.StreamStorage,
	merkleProofVerifier MerkleProofVerifier, feeQuoter FeeQuoter, key bitcoin.Key) *Wallet {
	return &Wallet{
		config:              config,
		store:               store,
		merkleProofVerifier: merkleProofVerifier,
		feeQuoter:           feeQuoter,
		baseKey:             key,
		keys:                make(KeySet),
	}
}

func (w *Wallet) BaseKey() bitcoin.Key {
	w.lock.RLock()
	result := w.baseKey
	w.lock.RUnlock()

	return result
}

func (w *Wallet) SetBlockHeight(ctx context.Context, blockHeight uint32) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.blockHeight = blockHeight
	return nil
}

func (w *Wallet) BlockHeight() uint32 {
	w.lock.RLock()
	defer w.lock.RUnlock()

	return w.blockHeight
}

func (w *Wallet) SetMerkleProofVerifier(m MerkleProofVerifier) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.merkleProofVerifier = m
}

// CreateBitcoinReceive creates a transaction receiving the specified amount of bitcoin.
func (w *Wallet) CreateBitcoinReceive(ctx context.Context, contextID bitcoin.Hash32,
	value uint64) (*channels.ExpandedTx, uint, error) {

	feeQuotes, err := w.feeQuoter.GetFeeQuotes(ctx)
	if err != nil {
		return nil, 0, errors.Wrap(err, "fee quotes")
	}

	keys, err := w.GenerateKeys(contextID, w.config.BreakCount)
	if err != nil {
		return nil, 0, errors.Wrap(err, "generate keys")
	}

	lockingScripts := make([]bitcoin.Script, len(keys))
	for i, key := range keys {
		lockingScripts[i] = key.LockingScript
	}

	standardFeeQuote := channels.GetFeeQuote(feeQuotes, merchant_api.FeeTypeStandard)
	outputs, err := txbuilder.BreakValueLockingScripts(value, w.config.SatoshiBreakValue,
		lockingScripts, standardFeeQuote.RelayFee.Rate(), standardFeeQuote.MiningFee.Rate(), false,
		false)
	if err != nil {
		return nil, 0, errors.Wrap(err, "break value")
	}

	etx := &channels.ExpandedTx{
		Tx: wire.NewMsgTx(1),
	}

	for _, output := range outputs {
		etx.Tx.AddTxOut(&output.TxOut)
	}

	depth, err := w.PopulateExpandedTx(ctx, etx)
	if err != nil {
		return nil, depth, errors.Wrap(err, "populate")
	}

	var keysUsed Keys
	for _, txout := range etx.Tx.TxOut {
		for _, key := range keys {
			if key.LockingScript.Equal(txout.LockingScript) {
				fmt.Printf("Using key %s : Hash %s\n", key.Key, key.Hash)
				keysUsed = append(keysUsed, key)
				break
			}
		}
	}

	if err := w.RetainKeys(contextID, keysUsed); err != nil {
		return nil, 0, errors.Wrap(err, "retain keys")
	}

	return etx, depth, nil
}

func (w *Wallet) FundTx(ctx context.Context, contextID bitcoin.Hash32,
	etx *channels.ExpandedTx, requirements channels.FeeRequirements) (uint, error) {

	fee := requirements.GetRequirement(merchant_api.FeeTypeStandard)
	if fee == nil {
		return 0, errors.New("Missing fee requirement")
	}
	miningFeeRate := fee.Rate()

	feeQuotes, err := w.feeQuoter.GetFeeQuotes(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "fee quotes")
	}
	standardFeeQuote := channels.GetFeeQuote(feeQuotes, merchant_api.FeeTypeStandard)
	relayFeeRate := standardFeeQuote.RelayFee.Rate()

	txb, err := txbuilder.NewTxBuilderFromWire(miningFeeRate, relayFeeRate, etx.Tx,
		etx.Ancestors.GetTxs())

	utxos, err := w.SelectUTXOs(ctx, contextID, etx)
	if err != nil {
		return 0, errors.Wrap(err, "utxos")
	}

	changeKeys, err := w.GenerateKeys(contextID, w.config.BreakCount)
	if err != nil {
		return 0, errors.Wrap(err, "change keys")
	}

	var changeAddresses []txbuilder.AddressKeyID
	for i, changeKey := range changeKeys {
		ra, err := bitcoin.RawAddressFromLockingScript(changeKey.LockingScript)
		if err != nil {
			return 0, errors.Wrapf(err, "change address %d", i)
		}

		changeAddresses = append(changeAddresses, txbuilder.AddressKeyID{
			Address: ra,
		})
	}

	if err := txb.AddFundingBreakChange(utxos, w.config.SatoshiBreakValue,
		changeAddresses); err != nil {
		return 0, errors.Wrap(err, "funding")
	}

	// Mark outputs as reserved
	w.outputsLock.Lock()
	outputsModified := false
	for _, txin := range txb.MsgTx.TxIn {
		for _, output := range w.outputs {
			if output.TxID.Equal(&txin.PreviousOutPoint.Hash) &&
				output.Index == txin.PreviousOutPoint.Index {
				output.ReservedContextID = &contextID
				outputsModified = true
			}
		}
	}

	if outputsModified {
		if err := w.outputs.save(ctx, w.store, w.BlockHeight()); err != nil {
			w.outputsLock.Unlock()
			return 0, errors.Wrap(err, "save outputs")
		}
	}
	w.outputsLock.Unlock()

	// Note: This can return MissingInputs or MissingMerkleProofAncestors that aren't necessarily
	// failures.
	depth, err := w.PopulateExpandedTx(ctx, etx)
	if err != nil {
		return depth, errors.Wrap(err, "populate")
	}

	var keysUsed Keys
	for _, txout := range etx.Tx.TxOut {
		for _, key := range changeKeys {
			if key.LockingScript.Equal(txout.LockingScript) {
				keysUsed = append(keysUsed, key)
				break
			}
		}
	}

	if err := w.RetainKeys(contextID, keysUsed); err != nil {
		return 0, errors.Wrap(err, "retain keys")
	}

	return depth, nil
}

func (w *Wallet) SignTx(ctx context.Context, contextID bitcoin.Hash32,
	etx *channels.ExpandedTx) error {

	keys, err := w.GetKeysForTx(ctx, contextID, etx)
	if err != nil {
		return errors.Wrap(err, "get keys")
	}

	fmt.Printf("Found %d keys\n", len(keys))
	for _, key := range keys {
		ra, _ := key.RawAddress()
		fmt.Printf("  Address : %s\n", bitcoin.NewAddressFromRawAddress(ra, bitcoin.MainNet))
	}

	feeQuotes, err := w.feeQuoter.GetFeeQuotes(ctx)
	if err != nil {
		return errors.Wrap(err, "fee quotes")
	}

	miningFeeRate := channels.GetFeeQuote(feeQuotes, merchant_api.FeeTypeStandard).MiningFee.Rate()
	relayFeeRate := channels.GetFeeQuote(feeQuotes, merchant_api.FeeTypeStandard).RelayFee.Rate()

	txb, err := txbuilder.NewTxBuilderFromWire(miningFeeRate, relayFeeRate, etx.Tx,
		etx.Ancestors.GetTxs())
	if err != nil {
		return errors.Wrap(err, "tx builder")
	}

	if _, err := txb.SignOnly(keys); err != nil {
		return errors.Wrap(err, "sign")
	}

	return nil
}

func (w *Wallet) SelectUTXOs(ctx context.Context, contextID bitcoin.Hash32,
	etx *channels.ExpandedTx) ([]bitcoin.UTXO, error) {
	w.outputsLock.RLock()
	defer w.outputsLock.RUnlock()

	var result []bitcoin.UTXO
	for _, output := range w.outputs {
		if output.SpentTxID != nil && output.ReservedContextID == nil &&
			output.State == TxStateSafe {
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

func (w *Wallet) AddTxWithoutContext(ctx context.Context, tx *wire.MsgTx) error {
	_, contextID, err := w.FindKeys(ctx, tx)
	if err != nil {
		return errors.Wrap(err, "find keys")
	}

	return w.AddTx(ctx, *contextID, tx)
}

func (w *Wallet) AddTx(ctx context.Context, contextID bitcoin.Hash32, tx *wire.MsgTx) error {
	txid := *tx.TxHash()

	keys, err := w.GetKeys(ctx, contextID)
	if err != nil {
		return errors.Wrap(err, "keys")
	}

	w.txLock.Lock()
	defer w.txLock.Unlock()

	// Check if tx was already added.
	walletTx, err := fetchTx(ctx, w.store, txid)
	if err != nil {
		return errors.Wrap(err, "fetch tx")
	}
	if walletTx != nil { // already added tx
		if walletTx.AddContextID(contextID) {
			logger.InfoWithFields(ctx, []logger.Field{
				logger.Stringer("txid", txid),
				logger.Stringer("context_id", contextID),
			}, "Added context id to existing tx")

			if err := walletTx.save(ctx, w.store); err != nil {
				return errors.Wrapf(err, "save %s", txid)
			}
		}

		return nil
	}

	// Check for spent outputs.
	w.outputsLock.Lock()
	for _, output := range w.outputs {
		for _, txin := range tx.TxIn {
			if !output.TxID.Equal(&txin.PreviousOutPoint.Hash) {
				continue
			}
			if output.Index != txin.PreviousOutPoint.Index {
				continue
			}

			if output.SpentTxID != nil {
				w.outputsLock.Unlock()
				return errors.Wrap(ErrOutputAlreadySpent, output.SpentTxID.String())
			}

			output.SpentTxID = &txid
			output.SpentHeight = w.blockHeight
			output.modified = true
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
				Timestamp:      channels.Now(),
				modified:       true,
			}
			copy(output.DerivationHash[:], key.Hash[:])

			logger.InfoWithFields(ctx, []logger.Field{
				logger.JSON("output", output),
			}, "New Output")
			w.outputs = append(w.outputs, output)
		}
	}

	if err := w.outputs.save(ctx, w.store, w.BlockHeight()); err != nil {
		w.outputsLock.Unlock()
		return errors.Wrap(err, "save outputs")
	}

	w.outputsLock.Unlock()

	w.keysLock.Lock()

	if err := w.keys.save(ctx, w.store, w.BlockHeight()); err != nil {
		w.keysLock.Unlock()
		return errors.Wrap(err, "save keys")
	}

	w.keysLock.Unlock()

	walletTx = &Tx{
		ContextIDs: []bitcoin.Hash32{contextID},
		Tx:         tx,
	}
	if err := walletTx.save(ctx, w.store); err != nil {
		return errors.Wrap(err, "save tx")
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("txid", tx.TxHash()),
		logger.Stringer("context_id", contextID),
	}, "Added new tx")

	return nil
}

// AddMerkleProof verifies the merkle proof and adds it to the tx if it doesn't have it already.
// It returns the context ids associated with the corresponding tx.
func (w *Wallet) AddMerkleProof(ctx context.Context,
	merkleProof *merkle_proof.MerkleProof) ([]bitcoin.Hash32, error) {

	txid := merkleProof.GetTxID()
	if txid == nil {
		return nil, errors.New("No txid in merkle proof")
	}

	if _, _, err := w.merkleProofVerifier.VerifyMerkleProof(ctx, merkleProof); err != nil {
		return nil, errors.Wrap(err, "verify")
	}

	w.txLock.Lock()
	defer w.txLock.Unlock()

	// Check if tx was already added.
	walletTx, err := fetchTx(ctx, w.store, *txid)
	if err != nil {
		return nil, errors.Wrap(err, "fetch tx")
	}
	if walletTx == nil {
		return nil, errors.Wrap(ErrUnknownTx, txid.String())
	}

	if err := walletTx.AddMerkleProof(ctx, merkleProof); err != nil {
		if errors.Cause(err) == AlreadyHaveMerkleProof {
			return nil, nil
		}
		return nil, errors.Wrap(err, "add merkle proof")
	}

	if err := walletTx.save(ctx, w.store); err != nil {
		return nil, errors.Wrap(err, "save tx")
	}

	return walletTx.ContextIDs, nil
}

func (w *Wallet) MarkTxSafe(ctx context.Context, txid bitcoin.Hash32) error {
	w.txLock.Lock()
	defer w.txLock.Unlock()

	// Check if tx was already added.
	walletTx, err := fetchTx(ctx, w.store, txid)
	if err != nil {
		return errors.Wrap(err, "fetch tx")
	}
	if walletTx == nil {
		return errors.Wrap(ErrUnknownTx, txid.String())
	}

	if walletTx.State == TxStateSafe {
		return nil
	}
	walletTx.State = TxStateSafe

	if err := walletTx.save(ctx, w.store); err != nil {
		return errors.Wrap(err, "save tx")
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("txid", txid),
	}, "Marked tx as safe")

	if err := w.markOutputs(ctx, txid, TxStateSafe); err != nil {
		return errors.Wrap(err, "utxos")
	}

	return nil
}

func (w *Wallet) MarkTxUnsafe(ctx context.Context, txid bitcoin.Hash32) error {
	w.txLock.Lock()
	defer w.txLock.Unlock()

	// Check if tx was already added.
	walletTx, err := fetchTx(ctx, w.store, txid)
	if err != nil {
		return errors.Wrap(err, "fetch tx")
	}
	if walletTx == nil {
		return errors.Wrap(ErrUnknownTx, txid.String())
	}

	if walletTx.State == TxStateUnsafe {
		return nil
	}
	walletTx.State = TxStateUnsafe

	if err := walletTx.save(ctx, w.store); err != nil {
		return errors.Wrap(err, "save tx")
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("txid", txid),
	}, "Marked tx as unsafe")

	if err := w.markOutputs(ctx, txid, TxStateUnsafe); err != nil {
		return errors.Wrap(err, "utxos")
	}

	return nil
}

func (w *Wallet) MarkTxCancelled(ctx context.Context, txid bitcoin.Hash32) error {
	w.txLock.Lock()
	defer w.txLock.Unlock()

	// Check if tx was already added.
	walletTx, err := fetchTx(ctx, w.store, txid)
	if err != nil {
		return errors.Wrap(err, "fetch tx")
	}
	if walletTx == nil {
		return errors.Wrap(ErrUnknownTx, txid.String())
	}

	if walletTx.State == TxStateCancelled {
		return nil
	}
	walletTx.State = TxStateCancelled

	if err := walletTx.save(ctx, w.store); err != nil {
		return errors.Wrap(err, "save tx")
	}

	logger.InfoWithFields(ctx, []logger.Field{
		logger.Stringer("txid", txid),
	}, "Marked tx as cancelled")

	if err := w.markOutputs(ctx, txid, TxStateCancelled); err != nil {
		return errors.Wrap(err, "utxos")
	}

	return nil
}

func (w *Wallet) markOutputs(ctx context.Context, txid bitcoin.Hash32, state TxState) error {
	w.outputsLock.Lock()
	defer w.outputsLock.Unlock()

	updated := false
	for _, output := range w.outputs {
		if output.TxID.Equal(&txid) && output.State != state {
			output.State = state
			output.modified = true
			updated = true

			logger.InfoWithFields(ctx, []logger.Field{
				logger.Stringer("txid", txid),
				logger.Uint32("index", output.Index),
				logger.Uint64("value", output.Value),
			}, "Marked output as %s", state)
		}
	}

	if updated {
		if err := w.outputs.save(ctx, w.store, w.BlockHeight()); err != nil {
			return errors.Wrap(err, "save outputs")
		}
	}

	return nil
}

func (w *Wallet) Load(ctx context.Context) error {
	path := fmt.Sprintf("%s/wallet", walletPath)
	if err := storage.StreamRead(ctx, w.store, path, w); err != nil {
		if errors.Cause(err) == storage.ErrNotFound {
			return nil
		}
		return errors.Wrap(err, "read")
	}

	w.outputsLock.Lock()
	if err := w.outputs.load(ctx, w.store); err != nil {
		w.outputsLock.Unlock()
		return errors.Wrap(err, "outputs")
	}
	w.outputsLock.Unlock()

	w.keysLock.Lock()
	if err := w.keys.load(ctx, w.store, w.BaseKey()); err != nil {
		w.keysLock.Unlock()
		return errors.Wrap(err, "keys")
	}
	w.keysLock.Unlock()

	return nil
}

func (w *Wallet) Save(ctx context.Context) error {
	path := fmt.Sprintf("%s/wallet", walletPath)
	if err := storage.StreamWrite(ctx, w.store, path, w); err != nil {
		return errors.Wrap(err, "write")
	}

	blockHeight := w.BlockHeight()

	w.outputsLock.RLock()
	if err := w.outputs.save(ctx, w.store, blockHeight); err != nil {
		w.outputsLock.RUnlock()
		return errors.Wrap(err, "outputs")
	}
	w.outputsLock.RUnlock()

	w.keysLock.RLock()
	if err := w.keys.save(ctx, w.store, blockHeight); err != nil {
		w.keysLock.RUnlock()
		return errors.Wrap(err, "keys")
	}
	w.keysLock.RUnlock()

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

	if err := binary.Write(writer, endian, w.blockHeight); err != nil {
		return errors.Wrap(err, "block height")
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

	if err := binary.Read(r, endian, &w.blockHeight); err != nil {
		return errors.Wrap(err, "block height")
	}

	return nil
}
