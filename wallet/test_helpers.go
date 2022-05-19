package wallet

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/storage"
	"github.com/tokenized/pkg/txbuilder"
	"github.com/tokenized/pkg/wire"
)

func MockWallet() (*Wallet, *MockMerkleProofVerifier, *MockFeeQuoter) {
	config := Config{
		SatoshiBreakValue: 10000,
		BreakCount:        5,
	}

	merkleProofVerifier := NewMockMerkleProofVerifier()
	feeQuoter := NewMockFeeQuoter()

	key, err := bitcoin.GenerateKey(bitcoin.MainNet)
	if err != nil {
		panic(fmt.Sprintf("Failed to generate key : %s", err))
	}

	wallet := NewWallet(config, storage.NewMockStorage(), merkleProofVerifier, feeQuoter, key)

	return wallet, merkleProofVerifier, feeQuoter
}

func MockWalletWith(merkleProofVerifier MerkleProofVerifier, feeQuoter FeeQuoter) *Wallet {
	config := Config{
		SatoshiBreakValue: 10000,
		BreakCount:        5,
	}

	key, err := bitcoin.GenerateKey(bitcoin.MainNet)
	if err != nil {
		panic(fmt.Sprintf("Failed to generate key : %s", err))
	}

	return NewWallet(config, storage.NewMockStorage(), merkleProofVerifier, feeQuoter, key)
}

func MockReceiveTx(ctx context.Context, wallet *Wallet, contextID bitcoin.Hash32,
	outputValues ...uint64) *wire.MsgTx {

	keys, err := wallet.GenerateKeys(contextID, len(outputValues))
	if err != nil {
		panic(fmt.Sprintf("Failed to generate keys : %s", err))
	}

	tx := wire.NewMsgTx(1)
	for i, value := range outputValues {
		tx.AddTxOut(wire.NewTxOut(value, keys[i].LockingScript))
	}

	verifier, ok := wallet.merkleProofVerifier.(*MockMerkleProofVerifier)
	if !ok {
		panic("Wallet does not have mock merkle proof verifier")
	}

	proofs := verifier.MockMerkleProofs(*tx.TxHash())

	if err := wallet.AddTx(ctx, tx, contextID); err != nil {
		panic(fmt.Sprintf("Failed to add input tx : %s", err))
	}

	if err := wallet.AddMerkleProof(ctx, proofs[0]); err != nil {
		panic(fmt.Sprintf("Failed to add input merkle proof : %s", err))
	}

	return tx
}

func MockUTXOs(ctx context.Context, wallet *Wallet, values ...uint64) []*bitcoin.UTXO {
	feeQuotes, err := wallet.feeQuoter.GetFeeQuotes(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to get fee quote : %s", err))
	}

	dustFeeRate := feeQuotes.GetQuote(channels.FeeTypeStandard).RelayFee.Rate()

	result := make([]*bitcoin.UTXO, len(values))
	for i, value := range values {
		contextID := RandomHash()

		// Create a receive of bitcoin
		etx := &channels.ExpandedTx{
			Tx: MockReceiveTx(ctx, wallet, contextID, value),
		}

		// Create inputs
		totalAmount := uint64(int(value*2) + rand.Intn(int(value*2)))
		remainingAmount := totalAmount
		var inputKeys []bitcoin.Key
		var inputLockingScripts []bitcoin.Script
		var inputAmounts []uint64
		for {
			key, err := bitcoin.GenerateKey(bitcoin.MainNet)
			if err != nil {
				panic(fmt.Sprintf("Failed to generate key : %s", err))
			}
			inputKeys = append(inputKeys, key)

			lockingScript, err := key.LockingScript()
			if err != nil {
				panic(fmt.Sprintf("Failed to create locking script : %s", err))
			}
			inputLockingScripts = append(inputLockingScripts, lockingScript)

			dust := txbuilder.DustLimitForLockingScript(lockingScript, dustFeeRate)

			amount := dust + uint64(rand.Intn(int(totalAmount/2)))
			if amount > remainingAmount {
				amount = remainingAmount
			}
			inputAmounts = append(inputAmounts, amount)

			inputTx := wire.NewMsgTx(1)
			inputTx.AddTxOut(wire.NewTxOut(amount, lockingScript))

			if amount > remainingAmount && amount-remainingAmount >= dust {
				// add change
				changeKey, err := bitcoin.GenerateKey(bitcoin.MainNet)
				if err != nil {
					panic(fmt.Sprintf("Failed to generate change key : %s", err))
				}

				changeLockingScript, err := changeKey.LockingScript()
				if err != nil {
					panic(fmt.Sprintf("Failed to create change locking script : %s", err))
				}

				inputTx.AddTxOut(wire.NewTxOut(amount-remainingAmount, changeLockingScript))
			}

			verifier, ok := wallet.merkleProofVerifier.(*MockMerkleProofVerifier)
			if !ok {
				panic("Wallet does not have mock merkle proof verifier")
			}

			proofs := verifier.MockMerkleProofs(*inputTx.TxHash())

			etx.Tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(inputTx.TxHash(), 0), nil))

			if err := wallet.AddTx(ctx, inputTx, contextID); err != nil {
				panic(fmt.Sprintf("Failed to add input tx : %s", err))
			}

			if err := wallet.AddMerkleProof(ctx, proofs[0]); err != nil {
				panic(fmt.Sprintf("Failed to add input merkle proof : %s", err))
			}

			if amount >= remainingAmount {
				break
			}

			remainingAmount -= amount
		}

		hashCache := &txbuilder.SigHashCache{}
		for i, key := range inputKeys {
			unlockingScript, err := txbuilder.P2PKHUnlockingScript(key, etx.Tx, 0,
				inputLockingScripts[i], inputAmounts[i],
				txbuilder.SigHashAll+txbuilder.SigHashForkID, hashCache)
			if err != nil {
				panic(fmt.Sprintf("Failed to create unlocking script : %s", err))
			}

			etx.Tx.TxIn[i].UnlockingScript = unlockingScript
		}

		if err := wallet.AddTx(ctx, etx.Tx, contextID); err != nil {
			panic(fmt.Sprintf("Failed to add tx : %s", err))
		}

		result[i] = &bitcoin.UTXO{
			Hash:          *etx.Tx.TxHash(),
			Index:         0,
			Value:         value,
			LockingScript: etx.Tx.TxOut[0].LockingScript,
		}
	}

	return result
}