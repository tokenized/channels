package channels

import (
	"bytes"
	"fmt"
	"math"

	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

var (
	ErrInvalidCharacter = errors.New("Invalid Character")
	ErrTooManyDecimals  = errors.New("Too Many Decimals")
)

// Decimal represents a decimal number. It is designed to not have precision rounding errors for
// prices.
// TODO Implement multiplication, division, addition, and subtraction. --ce
type Decimal struct {
	value     uint64
	precision uint8
}

func NewDecimal(value uint64, precision uint8) Decimal {
	return Decimal{
		value:     value,
		precision: precision,
	}
}

func (d *Decimal) Set(value uint64, precision uint8) {
	d.value = value
	d.precision = precision
}

func (d Decimal) Equal(other Decimal) bool {
	// TODO Add adjustment of different precisions with the same value. --ce

	if d.value != other.value {
		return false
	}

	if d.precision != other.precision {
		return false
	}

	return true
}

func (d Decimal) String() string {
	if d.precision == 0 {
		return fmt.Sprintf("%d", d.value)
	}

	formatter := fmt.Sprintf("%%0%dd", d.precision+1)
	s := fmt.Sprintf(formatter, d.value)

	i := uint(len(s)) - uint(d.precision)
	return s[:i] + "." + s[i:]
}

func (d *Decimal) SetString(s string) error {
	var value uint64
	var precision uint8
	decimalFound := false
	l := len(s)
	for i := 0; i < l; i++ {
		c := s[i]
		if c == '.' {
			if decimalFound {
				return ErrTooManyDecimals
			}

			precision = uint8(l - i - 1)
			continue
		}

		if c > '9' || c < '0' {
			return errors.Wrap(ErrInvalidCharacter, string([]byte{c}))
		}

		v := uint64(c - '0')
		value *= 10
		value += v
	}

	d.value = value
	d.precision = precision
	return nil
}

func (d Decimal) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", d)), nil
}

func (d *Decimal) UnmarshalJSON(data []byte) error {
	l := len(data)
	if l < 2 {
		return errors.New("Missing Quotes")
	}
	if data[0] != '"' || data[l-1] != '"' {
		return errors.New("Missing Quotes")
	}

	if err := d.SetString(string(data[1 : l-1])); err != nil {
		return err
	}

	return nil
}

func (d Decimal) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

func (d *Decimal) UnmarshalText(text []byte) error {
	return d.SetString(string(text))
}

func (d Decimal) MarshalBinary() ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := wire.WriteVarInt(buf, 0, d.value); err != nil {
		return nil, errors.Wrap(err, "value")
	}

	if err := wire.WriteVarInt(buf, 0, uint64(d.precision)); err != nil {
		return nil, errors.Wrap(err, "precision")
	}

	return buf.Bytes(), nil
}

func (d *Decimal) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	value, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return errors.Wrap(err, "value")
	}

	precision, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return errors.Wrap(err, "precision")
	}

	if precision > math.MaxUint8 {
		return fmt.Errorf("Overloaded precision : %d", precision)
	}

	d.value = value
	d.precision = uint8(precision)
	return nil
}
