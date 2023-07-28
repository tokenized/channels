package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/peer_channels"

	"github.com/pkg/errors"
)

const (
	ReplyToVersion = uint8(0)
)

var (
	ProtocolIDReplyTo = envelope.ProtocolID("RT") // Protocol ID for channel reply messages
)

type ReplyToProtocol struct{}

func NewReplyToProtocol() *ReplyToProtocol {
	return &ReplyToProtocol{}
}

func (*ReplyToProtocol) ProtocolID() envelope.ProtocolID {
	return ProtocolIDReplyTo
}

func (*ReplyToProtocol) Parse(payload envelope.Data) (Message, envelope.Data, error) {
	return ParseReplyTo(payload)
}

func (*ReplyToProtocol) ResponseCodeToString(code uint32) string {
	return ReplyToResponseCodeToString(code)
}

// ReplyTo is used to identify that a message is in reply to a previous message.
type ReplyTo struct {
	PeerChannel *peer_channels.Channel `bsor:"1" json:"peer_channel,omitempty"`
	Handle      *string                `bsor:"2" json:"handle,omitempty"`
}

func (r ReplyTo) String() string {
	if r.Handle != nil {
		return fmt.Sprintf("handle:%s", *r.Handle)
	}

	if r.PeerChannel != nil {
		return fmt.Sprintf("peer_channel:%s", r.PeerChannel.MaskedString())
	}

	return ""
}

func (*ReplyTo) IsWrapperType() {}

func (*ReplyTo) ProtocolID() envelope.ProtocolID {
	return ProtocolIDReplyTo
}

func (r *ReplyTo) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(ReplyToVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDReplyTo}, payload}, nil
}

func (r *ReplyTo) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(ReplyToVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	scriptItems = append(scriptItems, msgScriptItems...)

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDReplyTo}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func (r ReplyTo) Copy() ReplyTo {
	result := ReplyTo{}

	if r.PeerChannel != nil {
		c := r.PeerChannel.Copy()
		result.PeerChannel = &c
	}

	if r.Handle != nil {
		c := CopyString(*r.Handle)
		result.Handle = &c
	}

	return result
}

func CopyString(s string) string {
	result := make([]byte, len(s))
	copy(result, s)
	return string(result)
}

func ParseReplyTo(payload envelope.Data) (*ReplyTo, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 || !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDReplyTo) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 2 {
		return nil, payload, errors.Wrapf(ErrInvalidMessage, "not enough reply push ops: %d",
			len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion,
			fmt.Sprintf("reply: %d", version))
	}

	result := &ReplyTo{}
	payloads, err := bsor.Unmarshal(payload.Payload[1:], result)
	if err != nil {
		return nil, payload, errors.Wrap(err, "unmarshal")
	}
	payload.Payload = payloads

	return result, payload, nil
}

func ReplyToResponseCodeToString(code uint32) string {
	switch code {
	default:
		return "parse_error"
	}
}
