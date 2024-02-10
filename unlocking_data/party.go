package unlocking_data

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	PartyInitiator    = Party(0)
	PartyCounterParty = Party(1)
	PartyAny          = Party(math.MaxUint64)
)

type Party uint64

func OppositeParty(party Party) Party {
	if party == PartyInitiator {
		return PartyCounterParty
	} else {
		return PartyInitiator
	}
}

func (v *Party) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for party value : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v Party) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", v)), nil
}

func (v Party) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

func (v *Party) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *Party) SetString(value string) error {
	switch value {
	case "initiator":
		*v = PartyInitiator
	case "counterparty":
		*v = PartyCounterParty
	case "any":
		*v = PartyAny
	default:
		if strings.HasPrefix(value, "party_") {
			n, err := strconv.ParseUint(value[6:], 10, 64)
			if err == nil {
				*v = Party(n)
			}
		}

		return fmt.Errorf("Unknown party value \"%s\"", value)
	}

	return nil
}

func (v Party) String() string {
	switch v {
	case PartyInitiator:
		return "initiator"
	case PartyCounterParty:
		return "counterparty"
	case PartyAny:
		return "any"
	default:
		return fmt.Sprintf("party_%d", v)
	}
}
