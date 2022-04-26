package channels

import (
	"bytes"
	"fmt"
	"reflect"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"

	"github.com/pkg/errors"
)

var (
	ProtocolIDChannels = envelope.ProtocolID("C") // Protocol ID for general channel messages
	ChannelsVersion    = uint8(0)

	ChannelsMessageTypeInvalid = ChannelsMessageType(0)

	// ChannelsMessageTypeUnsupportedProtocol is the response to any message containing a protocol
	// id that is not supported.
	ChannelsMessageTypeUnsupportedProtocol = ChannelsMessageType(1)

	ErrNotChannels                = errors.New("Not Channels")
	ErrUnsupportedChannelsVersion = errors.New("Unsupported Channels Version")
	ErrUnsupportedChannelsMessage = errors.New("Unsupported Channels Message")
)

type ChannelsMessageType uint8

type UnsupportedProtocol struct {
	ProtocolID envelope.ProtocolID `bsor:"1" json:"protocol_id"`
}

func WriteChannels(message interface{}) (envelope.ProtocolIDs, bitcoin.ScriptItems, error) {
	msgType := ChannelsMessageTypeFor(message)
	if msgType == ChannelsMessageTypeInvalid {
		return nil, nil, errors.Wrap(ErrUnsupportedChannelsMessage,
			reflect.TypeOf(message).Name())
	}

	var scriptItems bitcoin.ScriptItems

	// Version
	scriptItems = append(scriptItems, bitcoin.PushNumberScriptItem(int64(ChannelsVersion)))

	// Message type
	scriptItems = append(scriptItems, bitcoin.PushNumberScriptItem(int64(msgType)))

	// Message
	msgScriptItems, err := bsor.Marshal(message)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal")
	}
	scriptItems = append(scriptItems, msgScriptItems...)

	return envelope.ProtocolIDs{ProtocolIDChannels}, scriptItems, nil
}

func ParseChannels(protocolIDs envelope.ProtocolIDs,
	payload bitcoin.ScriptItems) (interface{}, error) {

	if len(protocolIDs) != 1 {
		return nil, errors.Wrapf(ErrNotInvoice, "only one protocol supported")
	}

	if !bytes.Equal(protocolIDs[0], ProtocolIDChannels) {
		return nil, errors.Wrapf(ErrNotInvoice, "wrong protocol id: %x", protocolIDs[0])
	}

	if len(payload) == 0 {
		return nil, errors.Wrapf(ErrNotChannels, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(ErrUnsupportedChannelsVersion, fmt.Sprintf("%d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload[1])
	if err != nil {
		return nil, errors.Wrap(err, "message type")
	}

	result := ChannelsMessageForType(ChannelsMessageType(messageType))
	if result == nil {
		return nil, errors.Wrap(ErrUnsupportedChannelsMessage,
			fmt.Sprintf("%d", ChannelsMessageType(messageType)))
	}

	if _, err := bsor.Unmarshal(payload[2:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}

func ChannelsMessageForType(messageType ChannelsMessageType) interface{} {
	switch messageType {
	case ChannelsMessageTypeUnsupportedProtocol:
		return &UnsupportedProtocol{}
	case ChannelsMessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func ChannelsMessageTypeFor(message interface{}) ChannelsMessageType {
	switch message.(type) {
	case *UnsupportedProtocol:
		return ChannelsMessageTypeUnsupportedProtocol
	default:
		return ChannelsMessageTypeInvalid
	}
}

func (v *ChannelsMessageType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for ChannelsMessageType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v ChannelsMessageType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v ChannelsMessageType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown ChannelsMessageType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *ChannelsMessageType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *ChannelsMessageType) SetString(s string) error {
	switch s {
	case "unsupported_protocol":
		*v = ChannelsMessageTypeUnsupportedProtocol
	default:
		*v = ChannelsMessageTypeInvalid
		return fmt.Errorf("Unknown ChannelsMessageType value \"%s\"", s)
	}

	return nil
}

func (v ChannelsMessageType) String() string {
	switch v {
	case ChannelsMessageTypeUnsupportedProtocol:
		return "unsupported_protocol"
	default:
		return ""
	}
}
