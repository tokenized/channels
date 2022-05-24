package wallet

import (
	"context"

	"github.com/tokenized/pkg/merchant_api"
)

type MockFeeQuoter struct {
	current merchant_api.FeeQuotes
}

func NewMockFeeQuoter() *MockFeeQuoter {
	return &MockFeeQuoter{
		current: merchant_api.FeeQuotes{
			{
				FeeType: merchant_api.FeeTypeStandard,
				MiningFee: merchant_api.Fee{
					Satoshis: 250,
					Bytes:    1000,
				},
				RelayFee: merchant_api.Fee{
					Satoshis: 100,
					Bytes:    1000,
				},
			},
			{
				FeeType: merchant_api.FeeTypeData,
				MiningFee: merchant_api.Fee{
					Satoshis: 100,
					Bytes:    1000,
				},
				RelayFee: merchant_api.Fee{
					Satoshis: 100,
					Bytes:    1000,
				},
			},
		},
	}
}

func (m *MockFeeQuoter) GetFeeQuotes(ctx context.Context) (merchant_api.FeeQuotes, error) {
	return m.current, nil
}
