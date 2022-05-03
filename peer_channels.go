package channels

import (
	"bytes"
	"fmt"
	"reflect"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var (
	ProtocolIDPeerChannels = envelope.ProtocolID("peers") // Protocol ID for peer channel messages
	PeerChannelsVersion    = uint8(0)

	PeerChannelsMessageTypeInvalid       = PeerChannelsMessageType(0)
	PeerChannelsMessageTypeCreateChannel = PeerChannelsMessageType(1)
	PeerChannelsMessageTypeDeleteChannel = PeerChannelsMessageType(2)

	PeerChannelTypeStandard = PeerChannelType(0)
	PeerChannelTypePublic   = PeerChannelType(1)

	ErrNotPeerChannels                = errors.New("Not PeerChannels")
	ErrUnsupportedPeerChannelsVersion = errors.New("Unsupported PeerChannels Version")
	ErrUnsupportedPeerChannelsMessage = errors.New("Unsupported PeerChannels Message")
)

type PeerChannelsMessageType uint8
type PeerChannelType uint8

type CreateChannel struct {
	Type PeerChannelType `bsor:"1" json:"type"`
}

type DeleteChannel struct {
	ID uuid.UUID `bsor:"1" json:"id"`
}

func WritePeerChannel(message interface{}) (envelope.ProtocolIDs, bitcoin.ScriptItems, error) {
	msgType := PeerChannelsMessageTypeFor(message)
	if msgType == PeerChannelsMessageTypeInvalid {
		return nil, nil, errors.Wrap(ErrUnsupportedPeerChannelsMessage,
			reflect.TypeOf(message).Name())
	}

	var scriptItems bitcoin.ScriptItems

	// Version
	scriptItems = append(scriptItems, bitcoin.PushNumberScriptItem(int64(PeerChannelsVersion)))

	// Message type
	scriptItems = append(scriptItems, bitcoin.PushNumberScriptItem(int64(msgType)))

	// Message
	msgScriptItems, err := bsor.Marshal(message)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal")
	}
	scriptItems = append(scriptItems, msgScriptItems...)

	return envelope.ProtocolIDs{ProtocolIDPeerChannels}, scriptItems, nil
}

func ParsePeerChannel(protocolIDs envelope.ProtocolIDs,
	payload bitcoin.ScriptItems) (interface{}, error) {

	if len(protocolIDs) != 1 {
		return nil, errors.Wrapf(ErrNotInvoice, "only one protocol supported")
	}

	if !bytes.Equal(protocolIDs[0], ProtocolIDPeerChannels) {
		return nil, errors.Wrapf(ErrNotInvoice, "wrong protocol id: %x", protocolIDs[0])
	}

	if len(payload) == 0 {
		return nil, errors.Wrapf(ErrNotPeerChannels, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(ErrUnsupportedPeerChannelsVersion, fmt.Sprintf("%d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload[1])
	if err != nil {
		return nil, errors.Wrap(err, "message type")
	}

	result := PeerChannelsMessageForType(PeerChannelsMessageType(messageType))
	if result == nil {
		return nil, errors.Wrap(ErrUnsupportedPeerChannelsMessage,
			fmt.Sprintf("%d", PeerChannelsMessageType(messageType)))
	}

	if _, err := bsor.Unmarshal(payload[2:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}

func PeerChannelsMessageForType(messageType PeerChannelsMessageType) interface{} {
	switch messageType {
	case PeerChannelsMessageTypeCreateChannel:
		return &CreateChannel{}
	case PeerChannelsMessageTypeDeleteChannel:
		return &DeleteChannel{}
	case PeerChannelsMessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func PeerChannelsMessageTypeFor(message interface{}) PeerChannelsMessageType {
	switch message.(type) {
	case *CreateChannel:
		return PeerChannelsMessageTypeCreateChannel
	case *DeleteChannel:
		return PeerChannelsMessageTypeDeleteChannel
	default:
		return PeerChannelsMessageTypeInvalid
	}
}

func (v *PeerChannelsMessageType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for PeerChannelsMessageType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v PeerChannelsMessageType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v PeerChannelsMessageType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown PeerChannelsMessageType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *PeerChannelsMessageType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *PeerChannelsMessageType) SetString(s string) error {
	switch s {
	case "create":
		*v = PeerChannelsMessageTypeCreateChannel
	case "delete":
		*v = PeerChannelsMessageTypeDeleteChannel
	default:
		*v = PeerChannelsMessageTypeInvalid
		return fmt.Errorf("Unknown PeerChannelsMessageType value \"%s\"", s)
	}

	return nil
}

func (v PeerChannelsMessageType) String() string {
	switch v {
	case PeerChannelsMessageTypeCreateChannel:
		return "create"
	case PeerChannelsMessageTypeDeleteChannel:
		return "delete"
	default:
		return ""
	}
}

func (v *PeerChannelType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for PeerChannelType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v PeerChannelType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v PeerChannelType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown PeerChannelType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *PeerChannelType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *PeerChannelType) SetString(s string) error {
	switch s {
	case "standard":
		*v = PeerChannelTypeStandard
	case "public":
		*v = PeerChannelTypePublic
	default:
		*v = PeerChannelTypeStandard
		return fmt.Errorf("Unknown PeerChannelType value \"%s\"", s)
	}

	return nil
}

func (v PeerChannelType) String() string {
	switch v {
	case PeerChannelTypeStandard:
		return "standard"
	case PeerChannelTypePublic:
		return "public"
	default:
		return ""
	}
}
