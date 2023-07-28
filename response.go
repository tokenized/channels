package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"

	"github.com/pkg/errors"
)

const (
	ResponseVersion = uint8(0)

	// StatusOK means the request was valid and accepted.
	StatusOK = Status(0)

	// StatusReject means the request was rejected. The CodeProtocolID and Code should explain the
	// reason.
	StatusReject = Status(1)

	// StatusInvalid means something in the request was invalid. The CodeProtocolID and Code should
	// explain the reason.
	StatusInvalid = Status(2)

	// StatusUnauthorized means the request is not permitted.
	StatusUnauthorized = Status(3)

	// StatusUnsupportedProtocol means the message received used a protocol not supported by
	// this software.
	StatusUnsupportedProtocol = Status(4)

	// StatusUnwanted means the request message received was valid, but the recipient doesn't
	// want to accept it.
	StatusUnwanted = Status(5)

	// StatusNeedPayment means that a payment request was previously exchanged and not yet
	// fulfilled. Until that is fulfilled or renegotiated further requests will be rejected.
	StatusNeedPayment = Status(6)

	// StatusChannelInUse means the peer channel the request was received on is already in use
	// for another purpose.
	StatusChannelInUse = Status(7)

	// StatusSystemIssue means there was a systems issue and it was important to respond, but
	// a successful response was not possible.
	StatusSystemIssue = Status(8)

	ResponseStatusMessageNotFound = uint32(1)
	ResponseStatusWrongMessage    = uint32(2)
)

var (
	ProtocolIDResponse = envelope.ProtocolID("RE") // Protocol ID for channel response messages
)

type Status uint32

type ResponseProtocol struct{}

func NewResponseProtocol() *ResponseProtocol {
	return &ResponseProtocol{}
}

func (*ResponseProtocol) ProtocolID() envelope.ProtocolID {
	return ProtocolIDResponse
}

func (*ResponseProtocol) Parse(payload envelope.Data) (Message, envelope.Data, error) {
	return ParseResponse(payload)
}

func (*ResponseProtocol) ResponseCodeToString(code uint32) string {
	return ResponseStatusToString(code)
}

// Response is used to identify that a message is in response to a previous message.
type Response struct {
	Status         Status              `bsor:"2" json:"status,omitempty"`
	CodeProtocolID envelope.ProtocolID `bsor:"3" json:"protocol_id,omitempty"`
	Code           uint32              `bsor:"4" json:"code,omitempty"` // Protocol specific codes
	Note           string              `bsor:"5" json:"note,omitempty"`
}

func (r Response) Error() string {
	if len(r.Note) > 0 {
		return fmt.Sprintf("%s:%d %s", r.CodeProtocolID.String(), r.Code, r.Note)
	}
	return fmt.Sprintf("%s:%d", r.CodeProtocolID.String(), r.Code)
}

func (*Response) IsWrapperType() {}

func (*Response) ProtocolID() envelope.ProtocolID {
	return ProtocolIDResponse
}

func (r Response) Copy() Response {
	result := Response{
		Status: r.Status,
		Code:   r.Code,
		Note:   r.Note,
	}

	if len(r.CodeProtocolID) > 0 {
		result.CodeProtocolID = make(envelope.ProtocolID, len(r.CodeProtocolID))
		copy(result.CodeProtocolID[:], r.CodeProtocolID)
	}

	return result
}

func (r *Response) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(ResponseVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDResponse}, payload}, nil
}

func (r *Response) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(ResponseVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	scriptItems = append(scriptItems, msgScriptItems...)

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDResponse}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func ParseResponse(payload envelope.Data) (*Response, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 || !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDResponse) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 2 {
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

	result := &Response{}
	remainingPayload, err := bsor.Unmarshal(payload.Payload[1:], result)
	if err != nil {
		return nil, payload, errors.Wrap(err, "unmarshal")
	}
	payload.Payload = remainingPayload

	return result, payload, nil
}

func ResponseStatusToString(code uint32) string {
	switch code {
	case ResponseStatusMessageNotFound:
		return "message_not_found"
	case ResponseStatusWrongMessage:
		return "wrong_message"
	default:
		return "parse_error"
	}
}

func (v *Status) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for Status : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v Status) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v Status) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown Status value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *Status) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *Status) SetString(s string) error {
	switch s {
	case "ok":
		*v = StatusOK
	case "reject":
		*v = StatusReject
	case "invalid":
		*v = StatusInvalid
	case "unauthorized":
		*v = StatusUnauthorized
	case "unsupported_protocol":
		*v = StatusUnsupportedProtocol
	case "unwanted":
		*v = StatusUnwanted
	case "need_payment":
		*v = StatusNeedPayment
	case "in_use":
		*v = StatusChannelInUse
	case "system_issue":
		*v = StatusSystemIssue
	default:
		*v = StatusInvalid
		return fmt.Errorf("Unknown Status value \"%s\"", s)
	}

	return nil
}

func (v Status) String() string {
	switch v {
	case StatusOK:
		return "ok"
	case StatusReject:
		return "reject"
	case StatusInvalid:
		return "invalid"
	case StatusUnauthorized:
		return "unauthorized"
	case StatusUnsupportedProtocol:
		return "unsupported_protocol"
	case StatusUnwanted:
		return "unwanted"
	case StatusNeedPayment:
		return "need_payment"
	case StatusChannelInUse:
		return "in_use"
	case StatusSystemIssue:
		return "system_issue"
	default:
		return ""
	}
}
