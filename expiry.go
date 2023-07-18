package channels

import (
	"bytes"
	"fmt"

	"github.com/pkg/errors"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
)

const (
	ExpiryVersion = uint8(0)
)

var (
	ProtocolIDExpiry = envelope.ProtocolID("EXP") // Protocol ID for channel expirys
)

type ExpiryProtocol struct{}

// ExpiryMessage is a channels protocol message that contains a time.
type ExpiryMessage Time

func NewExpiryProtocol() *ExpiryProtocol {
	return &ExpiryProtocol{}
}

func (*ExpiryProtocol) ProtocolID() envelope.ProtocolID {
	return ProtocolIDExpiry
}

func (*ExpiryProtocol) Parse(payload envelope.Data) (Message, envelope.Data, error) {
	return ParseExpiry(payload)
}

func (*ExpiryProtocol) ResponseCodeToString(code uint32) string {
	return ExpiryResponseCodeToString(code)
}

func (m *ExpiryMessage) GetExpiry() Time {
	return Time(*m)
}

func NewExpiryMessage(t Time) *ExpiryMessage {
	cfr := ExpiryMessage(t)
	return &cfr
}

func (*ExpiryMessage) IsWrapperType() {}

func (*ExpiryMessage) ProtocolID() envelope.ProtocolID {
	return ProtocolIDExpiry
}

func (r *ExpiryMessage) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(ExpiryVersion))}

	// Message
	item := bitcoin.PushNumberScriptItemUnsigned(uint64(*r))
	payload = append(payload, item)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDExpiry}, payload}, nil
}

func (r *ExpiryMessage) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(ExpiryVersion))}

	// Message
	item := bitcoin.PushNumberScriptItemUnsigned(uint64(*r))
	scriptItems = append(scriptItems, item)

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDExpiry}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func ParseExpiry(payload envelope.Data) (*ExpiryMessage, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 ||
		!bytes.Equal(payload.ProtocolIDs[0], ProtocolIDExpiry) {
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
	result := ExpiryMessage(value)

	payload.Payload = payload.Payload[2:]

	return &result, payload, nil
}

func ExpiryResponseCodeToString(code uint32) string {
	switch code {
	default:
		return "parse_error"
	}
}
