package wallet

import (
	"github.com/tokenized/pkg/bitcoin"
)

const (
	outputsPath    = "channels_wallet/outputs"
	outputsVersion = uint8(0)
)

type Output struct {
	TxID           bitcoin.Hash32  `bsor:"1" json:"txid"`
	Index          uint32          `bsor:"2" json:"index"`
	Value          uint64          `bsor:"3" json:"value"`
	LockingScript  bitcoin.Script  `bsor:"4" json:"locking_script"`
	DerivationHash *bitcoin.Hash32 `bsor:"5" json:"derivation_hash`
	SpentTxID      *bitcoin.Hash32 `bsor:"6" json:"spent_txid"`
}

type Outputs []*Output
