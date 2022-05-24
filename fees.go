package channels

import "github.com/tokenized/pkg/merchant_api"

var (
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

func GetFeeQuote(feeQuotes merchant_api.FeeQuotes, t merchant_api.FeeType) *merchant_api.FeeQuote {
	result := feeQuotes.GetQuote(t)
	if result != nil {
		return result
	}

	return &DefaultFeeQuote
}
