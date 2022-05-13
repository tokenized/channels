package wallet

import (
	"context"

	"github.com/tokenized/channels"
)

type MockFeeQuoter struct {
	current channels.FeeQuotes
}

func NewMockFeeQuoter() *MockFeeQuoter {
	return &MockFeeQuoter{
		current: channels.FeeQuotes{
			{
				FeeType: channels.FeeQuoteTypeStandard,
				MiningFee: channels.Fee{
					Satoshis: 250,
					Bytes:    1000,
				},
				RelayFee: channels.Fee{
					Satoshis: 100,
					Bytes:    1000,
				},
			},
			{
				FeeType: channels.FeeQuoteTypeData,
				MiningFee: channels.Fee{
					Satoshis: 100,
					Bytes:    1000,
				},
				RelayFee: channels.Fee{
					Satoshis: 100,
					Bytes:    1000,
				},
			},
		},
	}
}

func (m *MockFeeQuoter) GetFeeQuotes(ctx context.Context) (channels.FeeQuotes, error) {
	return m.current, nil
}
