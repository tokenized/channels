package channels

import "fmt"

const (
	// FeeTypeStandard is for any bytes in the tx that don't fall in another fee type.
	FeeTypeStandard = FeeType(0)

	// FeeTypeData only applies to bytes in scripts that start with OP_RETURN or OP_FALSE, OP_RETURN.
	FeeTypeData = FeeType(1)
)

var (
	DefaultFeeQuote = FeeQuote{
		FeeType: FeeTypeStandard,
		MiningFee: Fee{
			Satoshis: 100,
			Bytes:    1000,
		},
		RelayFee: Fee{
			Satoshis: 50,
			Bytes:    1000,
		},
	}
)

type FeeType uint8

type FeeQuotes []*FeeQuote

type FeeQuote struct {
	FeeType   FeeType `bsor:"1" json:"feeType"`
	MiningFee Fee     `bsor:"2" json:"miningFee"`
	RelayFee  Fee     `bsor:"3" json:"relayFee"`
}

type Fee struct {
	Satoshis uint64 `bsor:"1" json:"satoshis"`
	Bytes    uint64 `bsor:"2" json:"bytes"`
}

func (f Fee) Rate() float32 {
	return float32(f.Satoshis) / float32(f.Bytes)
}

func (q FeeQuotes) GetQuote(t FeeType) FeeQuote {
	for _, quote := range q {
		if quote.FeeType == t {
			return *quote
		}
	}

	return DefaultFeeQuote
}

func (v *FeeType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for FeeType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v FeeType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v FeeType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown FeeType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *FeeType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *FeeType) SetString(s string) error {
	switch s {
	case "standard":
		*v = FeeTypeStandard
	case "data":
		*v = FeeTypeData
	default:
		*v = FeeTypeStandard
		return fmt.Errorf("Unknown FeeType value \"%s\"", s)
	}

	return nil
}

func (v FeeType) String() string {
	switch v {
	case FeeTypeStandard:
		return "standard"
	case FeeTypeData:
		return "data"
	default:
		return ""
	}
}
