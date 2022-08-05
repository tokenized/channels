package channels

import (
	"fmt"
)

const (
	DirectionReceiving = Direction(1)
	DirectionSending   = Direction(2)
)

type Direction uint8

func (v Direction) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown Direction value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *Direction) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *Direction) SetString(s string) error {
	switch s {
	case "sending":
		*v = DirectionSending
	case "receiving":
		*v = DirectionReceiving
	default:
		*v = 0
		return fmt.Errorf("Unknown Direction value \"%s\"", s)
	}

	return nil
}

func (v Direction) String() string {
	switch v {
	case DirectionSending:
		return "sending"
	case DirectionReceiving:
		return "receiving"
	default:
		return fmt.Sprintf("invalid(%d)", uint8(v))
	}
}
