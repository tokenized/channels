package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const (
	UUIDVersion = uint8(0)
)

var (
	ProtocolIDUUID = envelope.ProtocolID("UUID") // Protocol ID for uuid

	ErrInvalidUUID = errors.New("Invalid UUID")
)

type UUID uuid.UUID

type UUIDProtocol struct{}

func NewUUIDProtocol() *UUIDProtocol {
	return &UUIDProtocol{}
}

func (*UUIDProtocol) ProtocolID() envelope.ProtocolID {
	return ProtocolIDUUID
}

func (*UUIDProtocol) Parse(payload envelope.Data) (Message, envelope.Data, error) {
	return ParseUUID(payload)
}

func (*UUIDProtocol) ResponseCodeToString(code uint32) string {
	return SignedResponseCodeToString(code)
}

func (*UUID) IsWrapperType() {}

func (*UUID) ProtocolID() envelope.ProtocolID {
	return ProtocolIDUUID
}

func (m *UUID) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(UUIDVersion))}

	// Message
	payload = append(payload, bitcoin.NewPushDataScriptItem(m[:]))

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDUUID}, payload}, nil
}

// WrapUUID wraps the payload with the uuid and returns the new payload containing the uuid.
func WrapUUID(payload envelope.Data, id uuid.UUID) (envelope.Data, error) {
	uid := UUID(id)
	return uid.Wrap(payload)
}

func (m *UUID) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(UUIDVersion))}

	// Message
	scriptItems = append(scriptItems, bitcoin.NewPushDataScriptItem(m[:]))

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDUUID}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func ParseUUID(payload envelope.Data) (*UUID, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 || !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDUUID) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 2 {
		return nil, payload, errors.Wrapf(ErrInvalidMessage, "not enough uuid push ops: %d",
			len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion, fmt.Sprintf("uuid: %d", version))
	}

	if payload.Payload[1].Type != bitcoin.ScriptItemTypePushData {
		return nil, payload, errors.Wrap(ErrInvalidUUID, "not push data")
	}

	var result UUID
	if len(payload.Payload[1].Data) != len(result[:]) {
		return nil, payload, errors.Wrapf(ErrInvalidUUID, "wrong size: %d",
			len(payload.Payload[1].Data))
	}
	copy(result[:], payload.Payload[1].Data)

	payload.Payload = payload.Payload[2:]
	return &result, payload, nil
}
