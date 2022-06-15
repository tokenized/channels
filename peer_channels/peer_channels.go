package peer_channels

import (
	"bytes"
	"fmt"

	"github.com/tokenized/channels"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var (
	ProtocolID = envelope.ProtocolID("peers") // Protocol ID for peer channel messages
	Version    = uint8(0)

	MessageTypeInvalid       = MessageType(0)
	MessageTypeAccount       = MessageType(1)
	MessageTypeChannel       = MessageType(2)
	MessageTypeCreateChannel = MessageType(10)
	MessageTypeDeleteChannel = MessageType(11)

	ChannelTypeStandard = ChannelType(0)
	ChannelTypePublic   = ChannelType(1)

	ErrUnsupportedVersion             = errors.New("Unsupported PeerChannels Version")
	ErrUnsupportedPeerChannelsMessage = errors.New("Unsupported PeerChannels Message")
)

type MessageType uint8
type ChannelType uint8

type Protocol struct{}

func NewProtocol() *Protocol {
	return &Protocol{}
}

func (*Protocol) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (*Protocol) Parse(payload envelope.Data) (channels.Message, error) {
	return Parse(payload)
}

func (*Protocol) ResponseCodeToString(code uint32) string {
	return ResponseCodeToString(code)
}

func ResponseCodeToString(code uint32) string {
	return "unknown" // no response codes defined
}

// CalculatePeerChannelsServiceChannelID calculates the channel id that will be used by the peer
// channel service based on the public key with which the relationship is initiated. This allows a
// user with no peer channels service to initiate a relationship and have a peer channel to pay the
// initial invoice. After initiation it is used as the service channel and allows authorization of
// peer channel service actions on the account via Channels messages.
func CalculatePeerChannelsServiceChannelID(publicKey bitcoin.PublicKey) string {
	var channelID uuid.UUID
	publicKeyHash := bitcoin.Hash160(publicKey.Bytes())
	copy(channelID[:], publicKeyHash)
	return channelID.String()
}

func CalculatePeerChannelsServiceChannelToken(publicKey bitcoin.PublicKey) string {
	var token uuid.UUID
	copy(token[:], publicKey.Bytes())
	return token.String()
}

type Account struct {
	BaseURL string `bsor:"1" json:"base_url"`
	ID      string `bsor:"2" json:"id"`
	Token   string `bsor:"3" json:"token"`
}

func (*Account) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *Account) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeAccount)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

type Channel struct {
	BaseURL    string `bsor:"1" json:"base_url"`
	ID         string `bsor:"2" json:"id"`
	ReadToken  string `bsor:"3" json:"read_token"`
	WriteToken string `bsor:"4" json:"write_token"`
}

func (*Channel) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *Channel) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeChannel)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

type CreateChannel struct {
	Type ChannelType `bsor:"1" json:"type"`
}

func (*CreateChannel) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *CreateChannel) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(MessageTypeCreateChannel)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

type DeleteChannel struct {
	ID uuid.UUID `bsor:"1" json:"id"`
}

func (*DeleteChannel) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *DeleteChannel) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(MessageTypeDeleteChannel)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

func Parse(payload envelope.Data) (channels.Message, error) {
	if len(payload.ProtocolIDs) == 0 {
		return nil, nil
	}

	if !bytes.Equal(payload.ProtocolIDs[0], ProtocolID) {
		return nil, nil
	}

	if len(payload.ProtocolIDs) != 1 {
		return nil, errors.Wrapf(channels.ErrInvalidMessage, "peer channels can't wrap")
	}

	if len(payload.Payload) == 0 {
		return nil, errors.Wrapf(channels.ErrInvalidMessage, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(ErrUnsupportedVersion, fmt.Sprintf("%d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload.Payload[1])
	if err != nil {
		return nil, errors.Wrap(err, "message type")
	}

	result := MessageForType(MessageType(messageType))
	if result == nil {
		return nil, errors.Wrap(ErrUnsupportedPeerChannelsMessage,
			fmt.Sprintf("%d", MessageType(messageType)))
	}

	if _, err := bsor.Unmarshal(payload.Payload[2:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}

func MessageForType(messageType MessageType) channels.Message {
	switch messageType {
	case MessageTypeAccount:
		return &Account{}
	case MessageTypeChannel:
		return &Channel{}
	case MessageTypeCreateChannel:
		return &CreateChannel{}
	case MessageTypeDeleteChannel:
		return &DeleteChannel{}
	case MessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func MessageTypeFor(message channels.Message) MessageType {
	switch message.(type) {
	case *Account:
		return MessageTypeAccount
	case *CreateChannel:
		return MessageTypeCreateChannel
	case *DeleteChannel:
		return MessageTypeDeleteChannel
	case *Channel:
		return MessageTypeChannel
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
	case "account":
		*v = MessageTypeAccount
	case "channel":
		*v = MessageTypeChannel
	case "create":
		*v = MessageTypeCreateChannel
	case "delete":
		*v = MessageTypeDeleteChannel
	default:
		*v = MessageTypeInvalid
		return fmt.Errorf("Unknown MessageType value \"%s\"", s)
	}

	return nil
}

func (v MessageType) String() string {
	switch v {
	case MessageTypeAccount:
		return "account"
	case MessageTypeChannel:
		return "channel"
	case MessageTypeCreateChannel:
		return "create"
	case MessageTypeDeleteChannel:
		return "delete"
	default:
		return ""
	}
}

func (v *ChannelType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for ChannelType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v ChannelType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v ChannelType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown ChannelType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *ChannelType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *ChannelType) SetString(s string) error {
	switch s {
	case "standard":
		*v = ChannelTypeStandard
	case "public":
		*v = ChannelTypePublic
	default:
		*v = ChannelTypeStandard
		return fmt.Errorf("Unknown ChannelType value \"%s\"", s)
	}

	return nil
}

func (v ChannelType) String() string {
	switch v {
	case ChannelTypeStandard:
		return "standard"
	case ChannelTypePublic:
		return "public"
	default:
		return ""
	}
}
