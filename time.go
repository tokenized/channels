package channels

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
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

	TimeVersion = uint8(0)

	TimeFormat = "2006-01-02 15:04:05.999999999 -0700 MST"
)

var (
	ProtocolIDTime = envelope.ProtocolID("T") // Protocol ID for channel times
)

type PeriodType uint8
type Time uint64     // Nanoseconds since UNIX epoch
type Duration uint64 // Nanoseconds

type Period struct {
	Count uint64     `json:"count"`
	Type  PeriodType `json:"type"`
}

type TimeProtocol struct{}

// TimeMessage is a channels protocol message that contains a time.
type TimeMessage Time

func Now() Time {
	return Time(time.Now().UnixNano())
}

func ConvertToTime(t time.Time) Time {
	return Time(t.UnixNano())
}

func (t *Time) Add(d Duration) {
	*t += Time(d)
}

func (t *Time) Subtract(d Duration) {
	*t -= Time(d)
}

func (t Time) Copy() Time {
	return t
}

func ConvertToDuration(d time.Duration) Duration {
	return Duration(d.Nanoseconds())
}

func (t *Time) UnmarshalJSON(data []byte) error {
	v, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return err
	}

	*t = Time(v)
	return nil
}

func (t Time) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatUint(uint64(t), 10)), nil
}

func (t Time) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *Time) UnmarshalText(text []byte) error {
	return t.SetString(string(text))
}

func (t Time) String() string {
	tm := time.Unix(0, int64(t))
	return tm.Format(TimeFormat)
}

func (t *Time) SetString(text string) error {
	tm, err := time.Parse(TimeFormat, text)
	if err != nil {
		return err
	}

	*t = Time(tm.UnixNano())
	return nil
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

func NewTimeProtocol() *TimeProtocol {
	return &TimeProtocol{}
}

func (*TimeProtocol) ProtocolID() envelope.ProtocolID {
	return ProtocolIDTime
}

func (*TimeProtocol) Parse(payload envelope.Data) (Message, envelope.Data, error) {
	return ParseTime(payload)
}

func (*TimeProtocol) ResponseCodeToString(code uint32) string {
	return TimeResponseCodeToString(code)
}

func (m *TimeMessage) GetTime() Time {
	return Time(*m)
}

func NewTimeMessage(t Time) *TimeMessage {
	cfr := TimeMessage(t)
	return &cfr
}

func (*TimeMessage) IsWrapperType() {}

func (*TimeMessage) ProtocolID() envelope.ProtocolID {
	return ProtocolIDTime
}

func (r *TimeMessage) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(TimeVersion))}

	// Message
	item := bitcoin.PushNumberScriptItemUnsigned(uint64(*r))
	payload = append(payload, item)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDTime}, payload}, nil
}

func (r *TimeMessage) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(TimeVersion))}

	// Message
	item := bitcoin.PushNumberScriptItemUnsigned(uint64(*r))
	scriptItems = append(scriptItems, item)

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDTime}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func ParseTime(payload envelope.Data) (*TimeMessage, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 ||
		!bytes.Equal(payload.ProtocolIDs[0], ProtocolIDTime) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 2 {
		return nil, payload, errors.Wrapf(ErrInvalidMessage,
			"not enough fee requirements push ops: %d", len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion,
			fmt.Sprintf("fee requirements: %d", version))
	}

	value, err := bitcoin.ScriptNumberValueUnsigned(payload.Payload[1])
	if err != nil {
		return nil, payload, errors.Wrap(err, "value")
	}
	result := TimeMessage(value)

	payload.Payload = payload.Payload[2:]

	return &result, payload, nil
}

func TimeResponseCodeToString(code uint32) string {
	switch code {
	default:
		return "parse_error"
	}
}
