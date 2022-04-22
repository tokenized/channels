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
	ProtocolIDRelationships = envelope.ProtocolID("R") // Protocol ID for relationship messages
	RelationshipsVersion    = uint8(0)

	RelationshipsMessageTypeInvalid    = RelationshipsMessageType(0)
	RelationshipsMessageTypeInitiation = RelationshipsMessageType(1)
	RelationshipsMessageTypeAccept     = RelationshipsMessageType(2)

	ErrNotRelationships                = errors.New("Not Relationships")
	ErrUnsupportedRelationshipsVersion = errors.New("Unsupported Relationships Version")
	ErrUnsupportedRelationshipsMessage = errors.New("Unsupported Relationships Message")
)

type RelationshipsMessageType uint8

type RelationshipInitiation struct {
	PublicKey bitcoin.PublicKey `bsor:"1" json:"public_key"`
}

type RelationshipAccept struct {
	PublicKey bitcoin.PublicKey `bsor:"1" json:"public_key"`
}

func RelationshipsMessageForType(messageType RelationshipsMessageType) interface{} {
	switch messageType {
	case RelationshipsMessageTypeInitiation:
		return &RelationshipInitiation{}
	case RelationshipsMessageTypeAccept:
		return &RelationshipAccept{}
	case RelationshipsMessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func RelationshipsMessageTypeFor(message interface{}) RelationshipsMessageType {
	switch message.(type) {
	case *RelationshipInitiation:
		return RelationshipsMessageTypeInitiation
	case *RelationshipAccept:
		return RelationshipsMessageTypeAccept
	default:
		return RelationshipsMessageTypeInvalid
	}
}

func (v *RelationshipsMessageType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for RelationshipsMessageType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v RelationshipsMessageType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v RelationshipsMessageType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown RelationshipsMessageType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *RelationshipsMessageType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *RelationshipsMessageType) SetString(s string) error {
	switch s {
	case "initiation":
		*v = RelationshipsMessageTypeInitiation
	case "accept":
		*v = RelationshipsMessageTypeAccept
	default:
		*v = RelationshipsMessageTypeInvalid
		return fmt.Errorf("Unknown RelationshipsMessageType value \"%s\"", s)
	}

	return nil
}

func (v RelationshipsMessageType) String() string {
	switch v {
	case RelationshipsMessageTypeInitiation:
		return "initiation"
	case RelationshipsMessageTypeAccept:
		return "accept"
	default:
		return ""
	}
}

func WriteRelationships(message interface{}) (envelope.ProtocolIDs, bitcoin.ScriptItems, error) {
	msgType := RelationshipsMessageTypeFor(message)
	if msgType == RelationshipsMessageTypeInvalid {
		return nil, nil, errors.Wrap(ErrUnsupportedRelationshipsMessage,
			reflect.TypeOf(message).Name())
	}

	var scriptItems bitcoin.ScriptItems

	// Version
	scriptItems = append(scriptItems, bitcoin.PushNumberScriptItem(int64(RelationshipsVersion)))

	// Message type
	scriptItems = append(scriptItems, bitcoin.PushNumberScriptItem(int64(msgType)))

	// Message
	msgScriptItems, err := bsor.Marshal(message)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal")
	}
	scriptItems = append(scriptItems, msgScriptItems...)

	return envelope.ProtocolIDs{ProtocolIDRelationships}, scriptItems, nil
}

func ParseRelationships(protocolIDs envelope.ProtocolIDs,
	payload bitcoin.ScriptItems) (interface{}, error) {

	if len(protocolIDs) != 1 {
		return nil, errors.Wrapf(ErrNotInvoice, "only one protocol supported")
	}

	if !bytes.Equal(protocolIDs[0], ProtocolIDRelationships) {
		return nil, errors.Wrapf(ErrNotInvoice, "wrong protocol id: %x", protocolIDs[0])
	}

	if len(payload) == 0 {
		return nil, errors.Wrapf(ErrNotRelationships, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(ErrUnsupportedRelationshipsVersion, fmt.Sprintf("%d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload[1])
	if err != nil {
		return nil, errors.Wrap(err, "message type")
	}

	result := RelationshipsMessageForType(RelationshipsMessageType(messageType))
	if result == nil {
		return nil, errors.Wrap(ErrUnsupportedRelationshipsMessage,
			fmt.Sprintf("%d", RelationshipsMessageType(messageType)))
	}

	if _, err := bsor.Unmarshal(payload[2:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}
