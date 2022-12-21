package relationships

import (
	"bytes"
	"fmt"

	"github.com/tokenized/channels"
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
	Version = uint8(0)

	MessageTypeInvalid = MessageType(0)

	// MessageTypeInitiation initializes a relationship.
	MessageTypeInitiation = MessageType(1)

	// MessageTypeUpdate updates the configuration of the communication channel.
	MessageTypeUpdate = MessageType(2)

	// MessageTypeSubInitiation creates a new sub-channel that is part of this
	// relationship. It is the same identities involved, but a separate communication channel that
	// can be used by agents or for other purposes.
	MessageTypeSubInitiation = MessageType(3)

	// MessageTypeSubUpdate updates the configuration of sub-channel.
	MessageTypeSubUpdate = MessageType(4)

	// MessageTypeSubTerminate terminates a sub-channel. Neither party should expect
	// any further messages on the communication channels involved.
	MessageTypeSubTerminate = MessageType(5)

	StatusNotInitiated     = uint32(1)
	StatusAlreadyInitiated = uint32(2)
)

var (
	ProtocolID = envelope.ProtocolID("RS") // Protocol ID for relationship messages

	// OptionSubChannels specifies that sub channels are enabled. Sub-channels provide
	// separate channels of communication under one relationship. For example, an agent
	// administrator can establish a relationship with sub peer channels and a public key then
	// configure the agent to use the sub channels and public key. The invoices to pay for the
	// service will be sent to the primary peer channels, so the administrator can pay them, but the
	// agent can use the sub channels to access the service.
	OptionSubChannels = bitcoin.Hex{0x01}

	ErrUnsupportedRelationshipsMessage = errors.New("Unsupported Relationships Message")
)

type MessageType uint8

type ChannelConfiguration struct {
	// PublicKey is the base public key for a relationship. Channel message signing keys will be
	// derived from it.
	PublicKey bitcoin.PublicKey `bsor:"1" json:"public_key"`

	// PeerChannels for relationship to send messages to.
	PeerChannels channels.PeerChannels `bsor:"2" json:"peer_channels,omitempty"`

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

type Protocol struct{}

func NewProtocol() *Protocol {
	return &Protocol{}
}

func (*Protocol) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (*Protocol) Parse(payload envelope.Data) (channels.Message, envelope.Data, error) {
	return Parse(payload)
}

func (*Protocol) ResponseCodeToString(code uint32) string {
	return ResponseCodeToString(code)
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

type Initiation struct {
	Configuration ChannelConfiguration `bsor:"1" json:"data"`
	Identity      Identity             `bsor:"5" json:"identity"`
}

func (*Initiation) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (r *Initiation) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeInitiation)))

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

type Update struct {
	Configuration ChannelConfiguration `bsor:"1" json:"data"`
	Identity      Identity             `bsor:"5" json:"identity"`
}

func (*Update) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (r *Update) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeUpdate)))

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

type SubInitiation struct {
	Configuration ChannelConfiguration `bsor:"1" json:"data"`
}

func (*SubInitiation) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (r *SubInitiation) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(MessageTypeSubInitiation)))

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

type SubUpdate struct {
	Configuration ChannelConfiguration `bsor:"1" json:"data"`
}

func (*SubUpdate) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (r *SubUpdate) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(MessageTypeSubUpdate)))

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

type SubTerminate struct {
}

func (*SubTerminate) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (r *SubTerminate) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(MessageTypeSubTerminate)))

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

func Parse(payload envelope.Data) (channels.Message, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 {
		return nil, payload, nil
	}

	if !bytes.Equal(payload.ProtocolIDs[0], ProtocolID) {
		return nil, payload, nil
	}

	if len(payload.ProtocolIDs) != 1 {
		return nil, payload, errors.Wrapf(channels.ErrInvalidMessage, "relationship can't wrap")
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) == 0 {
		return nil, payload, errors.Wrapf(channels.ErrInvalidMessage, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(channels.ErrUnsupportedVersion,
			fmt.Sprintf("relationships: %d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload.Payload[1])
	if err != nil {
		return nil, payload, errors.Wrap(err, "message type")
	}

	result := MessageForType(MessageType(messageType))
	if result == nil {
		return nil, payload, errors.Wrap(ErrUnsupportedRelationshipsMessage,
			fmt.Sprintf("%d", MessageType(messageType)))
	}

	payloads, err := bsor.Unmarshal(payload.Payload[2:], result)
	if err != nil {
		return nil, payload, errors.Wrap(err, "unmarshal")
	}
	payload.Payload = payloads

	return result, payload, nil
}

func MessageForType(messageType MessageType) channels.Message {
	switch messageType {
	case MessageTypeInitiation:
		return &Initiation{}
	case MessageTypeUpdate:
		return &Update{}
	case MessageTypeSubInitiation:
		return &SubInitiation{}
	case MessageTypeSubUpdate:
		return &SubUpdate{}
	case MessageTypeSubTerminate:
		return &SubTerminate{}
	case MessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func MessageTypeFor(message channels.Message) MessageType {
	switch message.(type) {
	case *Initiation:
		return MessageTypeInitiation
	case *Update:
		return MessageTypeUpdate
	case *SubInitiation:
		return MessageTypeSubInitiation
	case *SubUpdate:
		return MessageTypeSubUpdate
	case *SubTerminate:
		return MessageTypeSubTerminate
	default:
		return MessageTypeInvalid
	}
}

func (v *MessageType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for MessageType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v MessageType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v MessageType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown MessageType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *MessageType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *MessageType) SetString(s string) error {
	switch s {
	case "initiation":
		*v = MessageTypeInitiation
	case "update":
		*v = MessageTypeUpdate
	case "sub-initiation":
		*v = MessageTypeSubInitiation
	case "sub-update":
		*v = MessageTypeSubUpdate
	case "sub-remove":
		*v = MessageTypeSubTerminate
	default:
		*v = MessageTypeInvalid
		return fmt.Errorf("Unknown MessageType value \"%s\"", s)
	}

	return nil
}

func (v MessageType) String() string {
	switch v {
	case MessageTypeInitiation:
		return "initiation"
	case MessageTypeUpdate:
		return "update"
	case MessageTypeSubInitiation:
		return "sub-initiation"
	case MessageTypeSubUpdate:
		return "sub-update"
	case MessageTypeSubTerminate:
		return "sub-remove"
	default:
		return ""
	}
}

func ResponseCodeToString(code uint32) string {
	switch code {
	case StatusNotInitiated:
		return "not_initiated"
	case StatusAlreadyInitiated:
		return "already_initiated"
	default:
		return "parse_error"
	}
}
