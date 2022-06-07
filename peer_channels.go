package channels

import (
	"bytes"
	"fmt"

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
	PeerChannelsMessageTypeAccount       = PeerChannelsMessageType(1)
	PeerChannelsMessageTypeCreateChannel = PeerChannelsMessageType(2)
	PeerChannelsMessageTypeDeleteChannel = PeerChannelsMessageType(3)

	PeerChannelTypeStandard = PeerChannelType(0)
	PeerChannelTypePublic   = PeerChannelType(1)

	ErrUnsupportedPeerChannelsVersion = errors.New("Unsupported PeerChannels Version")
	ErrUnsupportedPeerChannelsMessage = errors.New("Unsupported PeerChannels Message")
)

type PeerChannelsMessageType uint8
type PeerChannelType uint8

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

type PeerChannelsAccount struct {
	ID    string `bsor:"1" json:"id"`
	Token string `bsor:"2" json:"token"`
}

func (*PeerChannelsAccount) ProtocolID() envelope.ProtocolID {
	return ProtocolIDPeerChannels
}

func (m *PeerChannelsAccount) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(PeerChannelsVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(PeerChannelsMessageTypeAccount)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDPeerChannels}, payload}, nil
}

type PeerChannelsCreateChannel struct {
	Type PeerChannelType `bsor:"1" json:"type"`
}

func (*PeerChannelsCreateChannel) ProtocolID() envelope.ProtocolID {
	return ProtocolIDPeerChannels
}

func (m *PeerChannelsCreateChannel) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(PeerChannelsVersion))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(PeerChannelsMessageTypeCreateChannel)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDPeerChannels}, payload}, nil
}

type PeerChannelsDeleteChannel struct {
	ID uuid.UUID `bsor:"1" json:"id"`
}

func (*PeerChannelsDeleteChannel) ProtocolID() envelope.ProtocolID {
	return ProtocolIDPeerChannels
}

func (m *PeerChannelsDeleteChannel) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(PeerChannelsVersion))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(PeerChannelsMessageTypeDeleteChannel)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDPeerChannels}, payload}, nil
}

func ParsePeerChannels(payload envelope.Data) (Writer, error) {
	if len(payload.ProtocolIDs) == 0 {
		return nil, nil
	}

	if !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDPeerChannels) {
		return nil, nil
	}

	if len(payload.ProtocolIDs) != 1 {
		return nil, errors.Wrapf(ErrInvalidMessage, "peer channels can't wrap")
	}

	if len(payload.Payload) == 0 {
		return nil, errors.Wrapf(ErrInvalidMessage, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(ErrUnsupportedPeerChannelsVersion, fmt.Sprintf("%d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload.Payload[1])
	if err != nil {
		return nil, errors.Wrap(err, "message type")
	}

	result := PeerChannelsMessageForType(PeerChannelsMessageType(messageType))
	if result == nil {
		return nil, errors.Wrap(ErrUnsupportedPeerChannelsMessage,
			fmt.Sprintf("%d", PeerChannelsMessageType(messageType)))
	}

	if _, err := bsor.Unmarshal(payload.Payload[2:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}

func PeerChannelsMessageForType(messageType PeerChannelsMessageType) Writer {
	switch messageType {
	case PeerChannelsMessageTypeAccount:
		return &PeerChannelsAccount{}
	case PeerChannelsMessageTypeCreateChannel:
		return &PeerChannelsCreateChannel{}
	case PeerChannelsMessageTypeDeleteChannel:
		return &PeerChannelsDeleteChannel{}
	case PeerChannelsMessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func PeerChannelsMessageTypeFor(message Message) PeerChannelsMessageType {
	switch message.(type) {
	case *PeerChannelsAccount:
		return PeerChannelsMessageTypeAccount
	case *PeerChannelsCreateChannel:
		return PeerChannelsMessageTypeCreateChannel
	case *PeerChannelsDeleteChannel:
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
	case "account":
		*v = PeerChannelsMessageTypeAccount
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
	case PeerChannelsMessageTypeAccount:
		return "account"
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
