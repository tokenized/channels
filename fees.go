package channels

import (
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/merchant_api"
	"github.com/tokenized/pkg/wire"
)

var (
	DefaultFeeRequirement = FeeRequirement{
		FeeType:  merchant_api.FeeTypeStandard,
		Satoshis: 100,
		Bytes:    1000,
	}

	DefaultFeeRequirements = FeeRequirements{&DefaultFeeRequirement}

	DefaultFeeQuote = merchant_api.FeeQuote{
		FeeType: merchant_api.FeeTypeStandard,
		MiningFee: merchant_api.Fee{
			Satoshis: 100,
			Bytes:    1000,
		},
		RelayFee: merchant_api.Fee{
			Satoshis: 50,
			Bytes:    1000,
		},
	}
)

type FeeRequirement struct {
	FeeType  merchant_api.FeeType `bsor:"1" json:"feeType"`
	Satoshis uint64               `bsor:"2" json:"satoshis"`
	Bytes    uint64               `bsor:"3" json:"bytes"`
}

type FeeRequirements []*FeeRequirement

type FeeByteCount struct {
	FeeType merchant_api.FeeType `bsor:"1" json:"feeType"`
	Bytes   uint64               `bsor:"2" json:"bytes"`
}

type FeeByteCounts []*FeeByteCount

func GetFeeQuote(feeQuotes merchant_api.FeeQuotes, t merchant_api.FeeType) *merchant_api.FeeQuote {
	quote := feeQuotes.GetQuote(t)
	if quote != nil {
		return quote
	}

	return &DefaultFeeQuote
}

func (f FeeRequirement) Rate() float32 {
	return float32(f.Satoshis) / float32(f.Bytes)
}

func (reqs FeeRequirements) GetRequirement(t merchant_api.FeeType) *FeeRequirement {
	for _, req := range reqs {
		if req.FeeType == t {
			return req
		}
	}

	return nil
}

func ConvertToFeeRequirements(feeQuotes merchant_api.FeeQuotes) FeeRequirements {
	var result FeeRequirements
	for _, quote := range feeQuotes {
		result = append(result, &FeeRequirement{
			FeeType:  quote.FeeType,
			Satoshis: quote.MiningFee.Satoshis,
			Bytes:    quote.MiningFee.Bytes,
		})
	}

	return result
}

func TxFeeByteCounts(tx *wire.MsgTx) FeeByteCounts {
	standardByteCount := uint64(tx.SerializeSize())
	dataByteCount := uint64(0)

	for _, txout := range tx.TxOut {
		l := uint64(len(txout.LockingScript))
		if l < 2 {
			continue
		}

		if txout.LockingScript[0] == bitcoin.OP_RETURN ||
			(txout.LockingScript[0] == bitcoin.OP_FALSE &&
				txout.LockingScript[1] == bitcoin.OP_RETURN) {
			dataByteCount += l
			standardByteCount -= l
		}
	}

	return FeeByteCounts{
		{
			FeeType: merchant_api.FeeTypeStandard,
			Bytes:   standardByteCount,
		},
		{
			FeeType: merchant_api.FeeTypeData,
			Bytes:   dataByteCount,
		},
	}
}

func (reqs FeeRequirements) RequiredFee(bs FeeByteCounts) uint64 {
	feeRequired := uint64(0)
	for _, b := range bs {
		req := reqs.GetRequirement(b.FeeType)
		if req == nil {
			// Fall back to standard rate.
			req = reqs.GetStandardRequirement()
		}

		feeRequired += (b.Bytes * req.Satoshis) / req.Bytes
	}

	return feeRequired
}

func (reqs FeeRequirements) GetStandardRequirement() *FeeRequirement {
	req := reqs.GetRequirement(merchant_api.FeeTypeStandard)
	if req != nil {
		return req
	}

	// Fall back to default.
	return &DefaultFeeRequirement
}
