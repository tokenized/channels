package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

const (
	MessageIDVersion = uint8(0)
)

var (
	ProtocolIDMessageID = envelope.ProtocolID("ID") // Protocol ID for message id

	ErrInvalidMessageID = errors.New("Invalid MessageID")
)

type MessageID struct {
	MessageID uint64 `bsor:"-" json:"message_id"`
}

func (*MessageID) IsWrapperType() {}

func (*MessageID) ProtocolID() envelope.ProtocolID {
	return ProtocolIDMessageID
}

// WrapMessageID wraps the payload with the message id and returns the new payload containing the
// message id.
func WrapMessageID(payload envelope.Data, messageID uint64) (envelope.Data, error) {
	id := &MessageID{
		MessageID: messageID,
	}

	return id.Wrap(payload)
}

func (m *MessageID) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(MessageIDVersion))}

	// Message
	scriptItems = append(scriptItems, bitcoin.PushNumberScriptItemUnsigned(m.MessageID))

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDMessageID}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func ParseMessageID(payload envelope.Data) (*MessageID, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 || !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDMessageID) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 3 {
		return nil, payload, errors.Wrapf(ErrInvalidMessage, "not enough message id push ops: %d",
			len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion,
			fmt.Sprintf("message id: %d", version))
	}

	value, err := bitcoin.ScriptNumberValueUnsigned(payload.Payload[1])
	if err != nil {
		return nil, payload, errors.Wrap(err, "value")
	}
	result := &MessageID{
		MessageID: value,
	}

	payload.Payload = payload.Payload[2:]
	return result, payload, nil
}
