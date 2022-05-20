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
	RelationshipsVersion = uint8(0)

	RelationshipsMessageTypeInvalid    = RelationshipsMessageType(0)
	RelationshipsMessageTypeInitiation = RelationshipsMessageType(1)
	RelationshipsMessageTypeUpdate     = RelationshipsMessageType(2)

	RelationshipsRejectCodeNotInitiated     = uint32(1)
	RelationshipsRejectCodeAlreadyInitiated = uint32(2)
)

var (
	ProtocolIDRelationships = envelope.ProtocolID("RS") // Protocol ID for relationship messages

	ErrUnsupportedRelationshipsMessage = errors.New("Unsupported Relationships Message")
)

type RelationshipsMessageType uint8

type Entity struct {
	// PublicKey is the base public key for a relationship. Signature keys will be derived from it.
	PublicKey bitcoin.PublicKey `bsor:"1" json:"public_key"`

	// PeerChannels for relationship to send messages to.
	PeerChannels PeerChannels `bsor:"2" json:"peer_channel,omitempty"`

	// SupportedProtocols specifies the Envelope protocol IDs that can be interpreted by this
	// channel. If an unsuported protocol ID is used then this channel will respond with an
	// `UnsupportedProtocol` message.
	SupportedProtocols envelope.ProtocolIDs `bsor:"3" json:"supported_protocols"`

	Identity Identity `bsor:"4" json:"identity"`
}

type Identity struct {
	Name     *string   `bsor:"1" json:"name,omitempty"`
	Email    *string   `bsor:"2" json:"email,omitempty"`
	URL      *string   `bsor:"3" json:"url,omitempty"`
	Handle   *string   `bsor:"4" json:"handle,omitempty"`
	Phone    *string   `bsor:"5" json:"phone,omitempty"`
	Location *Location `bsor:"6" json:"location,omitempty"`
}

type Location struct {
	Streets    []string `bsor:"1" json:"streets"`
	City       string   `bsor:"2" json:"city"`
	Province   *string  `bsor:"3" json:"province,omitempty"` // State
	Country    *string  `bsor:"4" json:"country,omitempty"`
	PostalCode *string  `bsor:"5" json:"postal_code,omitempty"`
}

type RelationshipInitiation Entity

func (*RelationshipInitiation) ProtocolID() envelope.ProtocolID {
	return ProtocolIDRelationships
}

func (r *RelationshipInitiation) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(RelationshipsVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(RelationshipsMessageTypeInitiation)))

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDRelationships}, payload}, nil
}

type RelationshipUpdate Entity

func (*RelationshipUpdate) ProtocolID() envelope.ProtocolID {
	return ProtocolIDRelationships
}

func (r *RelationshipUpdate) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(RelationshipsVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(RelationshipsMessageTypeUpdate)))

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDRelationships}, payload}, nil
}

func ParseRelationship(payload envelope.Data) (Writer, error) {
	if len(payload.ProtocolIDs) == 0 {
		return nil, nil
	}

	if !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDRelationships) {
		return nil, nil
	}

	if len(payload.ProtocolIDs) != 1 {
		return nil, errors.Wrapf(ErrInvalidMessage, "relationship can't wrap")
	}

	if len(payload.Payload) == 0 {
		return nil, errors.Wrapf(ErrInvalidMessage, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(ErrUnsupportedVersion, fmt.Sprintf("relationships: %d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload.Payload[1])
	if err != nil {
		return nil, errors.Wrap(err, "message type")
	}

	result := RelationshipsMessageForType(RelationshipsMessageType(messageType))
	if result == nil {
		return nil, errors.Wrap(ErrUnsupportedRelationshipsMessage,
			fmt.Sprintf("%d", RelationshipsMessageType(messageType)))
	}

	if _, err := bsor.Unmarshal(payload.Payload[2:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}

func RelationshipsMessageForType(messageType RelationshipsMessageType) Writer {
	switch messageType {
	case RelationshipsMessageTypeInitiation:
		return &RelationshipInitiation{}
	case RelationshipsMessageTypeUpdate:
		return &RelationshipUpdate{}
	case RelationshipsMessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func RelationshipsMessageTypeFor(message Message) RelationshipsMessageType {
	switch message.(type) {
	case *RelationshipInitiation:
		return RelationshipsMessageTypeInitiation
	case *RelationshipUpdate:
		return RelationshipsMessageTypeUpdate
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
	case "update":
		*v = RelationshipsMessageTypeUpdate
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
	case RelationshipsMessageTypeUpdate:
		return "update"
	default:
		return ""
	}
}

func RelationshipsRejectCodeToString(code uint32) string {
	switch code {
	case RelationshipsRejectCodeNotInitiated:
		return "not_initiated"
	case RelationshipsRejectCodeAlreadyInitiated:
		return "already_initiated"
	default:
		return "parse_error"
	}
}
