package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

const (
	ResponseVersion = uint8(0)

	ResponseRejectCodeMessageNotFound = uint32(1)
)

var (
	ProtocolIDResponse = envelope.ProtocolID("RE") // Protocol ID for channel response messages
)

// Response is used to identify that a message is in response to a previous message.
type Response struct {
	MessageID uint64 `bsor:"-" json:"message_id"`
}

func (*Response) IsWrapperType() {}

func (*Response) ProtocolID() envelope.ProtocolID {
	return ProtocolIDResponse
}

func (r *Response) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(ResponseVersion))}

	// Message
	scriptItems = append(scriptItems, bitcoin.PushNumberScriptItemUnsigned(r.MessageID))

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDResponse}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func ParseResponse(payload envelope.Data) (*Response, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 || !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDResponse) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 3 {
		return nil, payload, errors.Wrapf(ErrInvalidMessage, "not enough response push ops: %d",
			len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion,
			fmt.Sprintf("response: %d", version))
	}

	value, err := bitcoin.ScriptNumberValueUnsigned(payload.Payload[1])
	if err != nil {
		return nil, payload, errors.Wrap(err, "value")
	}
	result := &Response{
		MessageID: value,
	}

	payload.Payload = payload.Payload[2:]
	return result, payload, nil
}

func ResponseRejectCodeToString(code uint32) string {
	switch code {
	case ResponseRejectCodeMessageNotFound:
		return "message_not_found"
	default:
		return "parse_error"
	}
}
