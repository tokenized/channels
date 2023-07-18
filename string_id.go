package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

const (
	StringIDVersion = uint8(0)
)

var (
	ProtocolIDStringID = envelope.ProtocolID("SID") // Protocol ID for string id

	ErrInvalidStringID = errors.New("Invalid StringID")
)

type StringIDProtocol struct{}

func NewStringIDProtocol() *StringIDProtocol {
	return &StringIDProtocol{}
}

func (*StringIDProtocol) ProtocolID() envelope.ProtocolID {
	return ProtocolIDStringID
}

func (*StringIDProtocol) Parse(payload envelope.Data) (Message, envelope.Data, error) {
	return ParseStringID(payload)
}

func (*StringIDProtocol) ResponseCodeToString(code uint32) string {
	return "parse"
}

type StringID struct {
	StringID string `bsor:"-" json:"thread_id"`
}

func NewStringID(id string) *StringID {
	return &StringID{StringID: id}
}

func (*StringID) IsWrapperType() {}

func (*StringID) ProtocolID() envelope.ProtocolID {
	return ProtocolIDStringID
}

// WrapStringID wraps the payload with the thread id and returns the new payload containing the
// thread id.
func WrapStringID(payload envelope.Data, threadID string) (envelope.Data, error) {
	id := &StringID{
		StringID: threadID,
	}

	return id.Wrap(payload)
}

func (m *StringID) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(StringIDVersion))}

	// Message
	scriptItems = append(scriptItems, bitcoin.NewPushDataScriptItem([]byte(m.StringID)))

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDStringID}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func ParseStringID(payload envelope.Data) (*StringID, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 || !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDStringID) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 3 {
		return nil, payload, errors.Wrapf(ErrInvalidMessage, "not enough thread id push ops: %d",
			len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion,
			fmt.Sprintf("thread id: %d", version))
	}

	if payload.Payload[1].Type != bitcoin.ScriptItemTypePushData {
		return nil, payload, errors.New("Not Push Data")
	}

	result := &StringID{
		StringID: string(payload.Payload[1].Data),
	}

	payload.Payload = payload.Payload[2:]
	return result, payload, nil
}
