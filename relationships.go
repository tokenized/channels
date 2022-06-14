package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"

	"github.com/pkg/errors"
)

// TODO Add proof of identity negotiation where identity oracles or other methods of verifying the
// identities given can be decided. Without that people must blindly trust the data or verify
// outside of this protocol. For example, if you initiate a channel with someone you know, you
// simply verify with them through other communication channels that they were on the other side.

const (
	RelationshipsVersion = uint8(0)

	RelationshipsMessageTypeInvalid = RelationshipsMessageType(0)

	// RelationshipsMessageTypeInitiation initializes a relationship.
	RelationshipsMessageTypeInitiation = RelationshipsMessageType(1)

	// RelationshipsMessageTypeUpdate updates the configuration of the communication channel.
	RelationshipsMessageTypeUpdate = RelationshipsMessageType(2)

	// RelationshipsMessageTypeSubInitiation creates a new sub-channel that is part of this
	// relationship. It is the same identities involved, but a separate communication channel that
	// can be used by agents or for other purposes.
	RelationshipsMessageTypeSubInitiation = RelationshipsMessageType(3)

	// RelationshipsMessageTypeSubUpdate updates the configuration of sub-channel.
	RelationshipsMessageTypeSubUpdate = RelationshipsMessageType(4)

	// RelationshipsMessageTypeSubTerminate terminates a sub-channel. Neither party should expect
	// any further messages on the communication channels involved.
	RelationshipsMessageTypeSubTerminate = RelationshipsMessageType(5)

	RelationshipsStatusNotInitiated     = uint32(1)
	RelationshipsStatusAlreadyInitiated = uint32(2)
)

var (
	ProtocolIDRelationships = envelope.ProtocolID("RS") // Protocol ID for relationship messages

	// RelationshipsOptionSubChannels specifies that sub channels are enabled. Sub-channels provide
	// separate channels of communication under one relationship. For example, an agent
	// administrator can establish a relationship with sub peer channels and a public key then
	// configure the agent to use the sub channels and public key. The invoices to pay for the
	// service will be sent to the primary peer channels, so the administrator can pay them, but the
	// agent can use the sub channels to access the service.
	RelationshipsOptionSubChannels = bitcoin.Hex{0x01}

	ErrUnsupportedRelationshipsMessage = errors.New("Unsupported Relationships Message")
)

type RelationshipsMessageType uint8

type ChannelConfiguration struct {
	// PublicKey is the base public key for a relationship. Channel message signing keys will be
	// derived from it.
	PublicKey bitcoin.PublicKey `bsor:"1" json:"public_key"`

	// PeerChannels for relationship to send messages to.
	PeerChannels PeerChannels `bsor:"2" json:"peer_channels,omitempty"`

	// SupportedProtocols specifies the Envelope protocol IDs that can be interpreted by this
	// channel. If an unsuported protocol ID is used then this channel will respond with an
	// `UnsupportedProtocol` message.
	SupportedProtocols envelope.ProtocolIDs `bsor:"3" json:"supported_protocols"`

	// ProtocolOptions allows specifying optional protocol specific options. Some
	// protocols may allow the implementation to enforce certain optional requirements or features.
	// For example, with the Invoices protocol, regarding ancestors, not all recipients will need to
	// require full ancestry to verify a tx. So if full ancestry or just direct parents are required
	// then that can be specified.
	ProtocolOptions ProtocolOptions `bsor:"4" json:"protocol_options"`
}

// ProtocolOption is an optional feature of a protocol that can be supported by an implementation or
// not. Providing these during relationship initiation helps both parties be aware of the other's
// specific requirements and abilities.
type ProtocolOption struct {
	Protocol envelope.ProtocolID `bsor:"1" json:"protocol"`
	Option   bitcoin.Hex         `bsor:"2" json:"option"`
}

type ProtocolOptions []*ProtocolOption

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

type RelationshipInitiation struct {
	Configuration ChannelConfiguration `bsor:"1" json:"data"`
	Identity      Identity             `bsor:"5" json:"identity"`
}

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

type RelationshipUpdate struct {
	Configuration ChannelConfiguration `bsor:"1" json:"data"`
	Identity      Identity             `bsor:"5" json:"identity"`
}

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

type RelationshipsSubInitiation struct {
	Configuration ChannelConfiguration `bsor:"1" json:"data"`
}

func (*RelationshipsSubInitiation) ProtocolID() envelope.ProtocolID {
	return ProtocolIDRelationships
}

func (r *RelationshipsSubInitiation) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(RelationshipsVersion))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(RelationshipsMessageTypeSubInitiation)))

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDRelationships}, payload}, nil
}

type RelationshipsSubUpdate struct {
	Configuration ChannelConfiguration `bsor:"1" json:"data"`
}

func (*RelationshipsSubUpdate) ProtocolID() envelope.ProtocolID {
	return ProtocolIDRelationships
}

func (r *RelationshipsSubUpdate) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(RelationshipsVersion))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(RelationshipsMessageTypeSubUpdate)))

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDRelationships}, payload}, nil
}

type RelationshipsSubTerminate struct {
}

func (*RelationshipsSubTerminate) ProtocolID() envelope.ProtocolID {
	return ProtocolIDRelationships
}

func (r *RelationshipsSubTerminate) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(RelationshipsVersion))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(RelationshipsMessageTypeSubTerminate)))

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
	case RelationshipsMessageTypeSubInitiation:
		return &RelationshipsSubInitiation{}
	case RelationshipsMessageTypeSubUpdate:
		return &RelationshipsSubUpdate{}
	case RelationshipsMessageTypeSubTerminate:
		return &RelationshipsSubTerminate{}
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
	case *RelationshipsSubInitiation:
		return RelationshipsMessageTypeSubInitiation
	case *RelationshipsSubUpdate:
		return RelationshipsMessageTypeSubUpdate
	case *RelationshipsSubTerminate:
		return RelationshipsMessageTypeSubTerminate
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
	case "sub-initiation":
		*v = RelationshipsMessageTypeSubInitiation
	case "sub-update":
		*v = RelationshipsMessageTypeSubUpdate
	case "sub-remove":
		*v = RelationshipsMessageTypeSubTerminate
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
	case RelationshipsMessageTypeSubInitiation:
		return "sub-initiation"
	case RelationshipsMessageTypeSubUpdate:
		return "sub-update"
	case RelationshipsMessageTypeSubTerminate:
		return "sub-remove"
	default:
		return ""
	}
}

func RelationshipsStatusToString(code uint32) string {
	switch code {
	case RelationshipsStatusNotInitiated:
		return "not_initiated"
	case RelationshipsStatusAlreadyInitiated:
		return "already_initiated"
	default:
		return "parse_error"
	}
}
