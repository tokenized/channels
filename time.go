package channels

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

const (
	PeriodTypeUnspecified = PeriodType(0)
	PeriodTypeSecond      = PeriodType(1)
	PeriodTypeMinute      = PeriodType(2)
	PeriodTypeHour        = PeriodType(3)
	PeriodTypeDay         = PeriodType(4)
	PeriodTypeWeek        = PeriodType(5)
	PeriodTypeMonth       = PeriodType(6)
	PeriodTypeYear        = PeriodType(7)
)

type PeriodType uint8
type Timestamp uint64 // Seconds since UNIX epoch
type Duration uint64  // Seconds

func Now() Timestamp {
	return Timestamp(time.Now().Unix())
}

func ConvertToTimestamp(t time.Time) Timestamp {
	return Timestamp(t.Unix())
}

func (t *Timestamp) Add(d Duration) {
	*t += Timestamp(d)
}

func ConvertToDuration(d time.Duration) Duration {
	return Duration(d.Seconds())
}

func (v *PeriodType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for PeriodType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v PeriodType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v PeriodType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown PeriodType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *PeriodType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *PeriodType) SetString(s string) error {
	switch s {
	case "second", "seconds":
		*v = PeriodTypeSecond
	case "minute", "minutes":
		*v = PeriodTypeMinute
	case "hour", "hours":
		*v = PeriodTypeHour
	case "day", "days":
		*v = PeriodTypeDay
	case "week", "weeks":
		*v = PeriodTypeWeek
	case "month", "months":
		*v = PeriodTypeMonth
	case "year", "years":
		*v = PeriodTypeYear
	default:
		*v = PeriodTypeUnspecified
		return fmt.Errorf("Unknown PeriodType value \"%s\"", s)
	}

	return nil
}

func (v PeriodType) String() string {
	switch v {
	case PeriodTypeSecond:
		return "second"
	case PeriodTypeMinute:
		return "minute"
	case PeriodTypeHour:
		return "hour"
	case PeriodTypeDay:
		return "day"
	case PeriodTypeWeek:
		return "week"
	case PeriodTypeMonth:
		return "month"
	case PeriodTypeYear:
		return "year"
	default:
		return ""
	}
}

func (v *Period) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for Period : %d", len(data))
	}

	if string(data) == "null" {
		v.Count = 0
		v.Type = PeriodTypeUnspecified
		return nil
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v Period) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v Period) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

func (v *Period) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *Period) SetString(s string) error {
	parts := strings.Split(s, " ")

	if len(parts) == 1 {
		v.Count = 1
		if err := v.Type.SetString(s); err != nil {
			return errors.Wrap(err, "type")
		}

		return nil
	}

	if len(parts) != 2 {
		return fmt.Errorf("Wrong Period spaces : got %d, want %d", len(parts), 2)
	}

	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return errors.Wrap(err, "count")
	}
	v.Count = uint64(count)

	if err := v.Type.SetString(parts[1]); err != nil {
		return errors.Wrap(err, "type")
	}

	return nil
}

func (v Period) String() string {
	if v.Count == 0 || v.Type == PeriodTypeUnspecified {
		return ""
	}
	if v.Count == 1 {
		return v.Type.String()
	}
	return fmt.Sprintf("%d %ss", v.Count, v.Type)
}

func (v Period) MarshalBinary() (data []byte, err error) {
	if v.Type == PeriodTypeUnspecified {
		return nil, nil
	}

	if v.Count == 1 {
		return []byte{byte(v.Type)}, nil
	}

	buf := &bytes.Buffer{}
	buf.WriteByte(byte(v.Type))
	if err := wire.WriteVarInt(buf, 0, v.Count); err != nil {
		return nil, errors.Wrap(err, "count")
	}

	return buf.Bytes(), nil
}

func (v *Period) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		v.Count = 0
		v.Type = PeriodTypeUnspecified
		return nil
	}

	if len(data) == 1 {
		v.Count = 1
		v.Type = PeriodType(data[0])
		return nil
	}

	if len(data) < 2 {
		v.Count = 0
		v.Type = PeriodTypeUnspecified
	}

	v.Type = PeriodType(data[0])
	count, err := wire.ReadVarInt(bytes.NewReader(data[1:]), 0)
	if err != nil {
		return errors.Wrap(err, "count")
	}
	v.Count = count
	return nil
}
