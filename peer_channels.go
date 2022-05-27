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
	PeerChannelsMessageTypeCreateChannel = PeerChannelsMessageType(1)
	PeerChannelsMessageTypeDeleteChannel = PeerChannelsMessageType(2)

	PeerChannelTypeStandard = PeerChannelType(0)
	PeerChannelTypePublic   = PeerChannelType(1)

	ErrUnsupportedPeerChannelsVersion = errors.New("Unsupported PeerChannels Version")
	ErrUnsupportedPeerChannelsMessage = errors.New("Unsupported PeerChannels Message")
)

type PeerChannelsMessageType uint8
type PeerChannelType uint8

// CalculatePeerChannelsAccount calculates the account id and token that will be used by the peer
// channel service based on the public key with which the relationship is initiated. This allows a
// user with no peer channels service to initiate a relationship and have a peer channel to pay the
// initial invoice. It also allows authorization of actions on the account via the signatures in the
// Channels messages.
func CalculatePeerChannelsAccount(publicKey bitcoin.PublicKey) (uuid.UUID, uuid.UUID) {
	var accountID, token uuid.UUID
	publicKeyHash := bitcoin.Hash160(publicKey.Bytes())
	copy(accountID[:], publicKeyHash)
	copy(token[:], publicKey.Bytes())
	return accountID, token
}

type CreateChannel struct {
	Type PeerChannelType `bsor:"1" json:"type"`
}

func (*CreateChannel) ProtocolID() envelope.ProtocolID {
	return ProtocolIDPeerChannels
}

func (m *CreateChannel) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(PeerChannelsVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(PeerChannelsMessageTypeCreateChannel)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDPeerChannels}, payload}, nil
}

type DeleteChannel struct {
	ID uuid.UUID `bsor:"1" json:"id"`
}

func (*DeleteChannel) ProtocolID() envelope.ProtocolID {
	return ProtocolIDPeerChannels
}

func (m *DeleteChannel) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(PeerChannelsVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(PeerChannelsMessageTypeDeleteChannel)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDPeerChannels}, payload}, nil
}

func ParsePeerChannel(payload envelope.Data) (Message, error) {
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

func PeerChannelsMessageForType(messageType PeerChannelsMessageType) Message {
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

func PeerChannelsMessageTypeFor(message Message) PeerChannelsMessageType {
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
