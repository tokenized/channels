package channels

import (
	"bytes"
	"fmt"
	"reflect"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/merkle_proof"

	"github.com/pkg/errors"
)

var (
	ProtocolIDChannels = envelope.ProtocolID("C") // Protocol ID for general channel messages
	ChannelsVersion    = uint8(0)

	ChannelsMessageTypeInvalid = ChannelsMessageType(0)

	// ChannelsMessageTypeUnsupportedProtocol is the response to any message containing a protocol
	// id that is not supported.
	ChannelsMessageTypeResponse    = ChannelsMessageType(1)
	ChannelsMessageTypeReject      = ChannelsMessageType(2)
	ChannelsMessageTypeMerkleProof = ChannelsMessageType(3)

	RejectReasonInvalid             = RejectReason(0)
	RejectReasonUnsupportedProtocol = RejectReason(1)

	// ResponsesRejectReasonChannelInUse means the peer channel this was received on is already
	// in use for a relationship.
	RejectReasonChannelInUse = RejectReason(2)
	RejectReasonUnwanted     = RejectReason(3)

	ErrNotChannels                = errors.New("Not Channels")
	ErrUnsupportedChannelsVersion = errors.New("Unsupported Channels Version")
	ErrUnsupportedChannelsMessage = errors.New("Unsupported Channels Message")
)

type ChannelsMessageType uint8
type RejectReason uint8

type UnsupportedProtocol struct {
	ProtocolID envelope.ProtocolID `bsor:"1" json:"protocol_id"`
}

type MerkleProof struct {
	MerkleProof merkle_proof.MerkleProof `bsor:"2" json:"merkle_proof"`
}

type Response struct {
	MessageHash bitcoin.Hash32 `bsor:"1" json:"message_hash"`
}

type Reject struct {
	Reason RejectReason `bsor:"1" json:"reason"`
	Note   string       `bsor:"2" json:"note"`
}

type Channel struct {
	Sent         *Entity      `bsor:"1" json:"sent"`
	Received     *Entity      `bsor:"2" json:"received"`
	PeerChannels PeerChannels `bsor:"3" json:"peer_channels"`
}

type Channels []*Channel

type PeerChannel struct {
	URL        string `bsor:"1" json:"url"`
	WriteToken string `bsor:"2" json:"write_token"`
}

type PeerChannels []PeerChannel

func WriteChannels(message interface{}, protocolIDs envelope.ProtocolIDs,
	payload bitcoin.ScriptItems) (envelope.ProtocolIDs, bitcoin.ScriptItems, error) {

	msgType := ChannelsMessageTypeFor(message)
	if msgType == ChannelsMessageTypeInvalid {
		return nil, nil, errors.Wrap(ErrUnsupportedChannelsMessage, reflect.TypeOf(message).Name())
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

	return append(envelope.ProtocolIDs{ProtocolIDChannels}, protocolIDs...),
		append(scriptItems, payload...), nil
}

func ParseChannels(protocolIDs envelope.ProtocolIDs,
	payload bitcoin.ScriptItems) (interface{}, envelope.ProtocolIDs, bitcoin.ScriptItems, error) {

	if len(protocolIDs) == 0 {
		return nil, nil, nil, errors.Wrapf(ErrNotChannels, "no protocol ids")
	}

	if !bytes.Equal(protocolIDs[0], ProtocolIDChannels) {
		return nil, nil, nil, errors.Wrapf(ErrNotChannels, "wrong protocol id: %x", protocolIDs[0])
	}

	if len(payload) == 0 {
		return nil, nil, nil, errors.Wrapf(ErrNotChannels, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload[0])
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, nil, nil, errors.Wrap(ErrUnsupportedChannelsVersion, fmt.Sprintf("%d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload[1])
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "message type")
	}

	result := ChannelsMessageForType(ChannelsMessageType(messageType))
	if result == nil {
		return nil, nil, nil, errors.Wrap(ErrUnsupportedChannelsMessage,
			fmt.Sprintf("%d", ChannelsMessageType(messageType)))
	}

	payload, err = bsor.Unmarshal(payload[2:], result)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "unmarshal")
	}

	return result, protocolIDs[1:], payload, nil
}

func ChannelsMessageForType(messageType ChannelsMessageType) interface{} {
	switch messageType {
	case ChannelsMessageTypeResponse:
		return &Response{}
	case ChannelsMessageTypeReject:
		return &Reject{}
	case ChannelsMessageTypeMerkleProof:
		return &MerkleProof{}
	case ChannelsMessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func ChannelsMessageTypeFor(message interface{}) ChannelsMessageType {
	switch message.(type) {
	case *Response:
		return ChannelsMessageTypeResponse
	case *Reject:
		return ChannelsMessageTypeReject
	case *MerkleProof:
		return ChannelsMessageTypeMerkleProof
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
	case "response":
		*v = ChannelsMessageTypeResponse
	case "reject":
		*v = ChannelsMessageTypeReject
	case "merkle_proof":
		*v = ChannelsMessageTypeMerkleProof
	default:
		*v = ChannelsMessageTypeInvalid
		return fmt.Errorf("Unknown ChannelsMessageType value \"%s\"", s)
	}

	return nil
}

func (v ChannelsMessageType) String() string {
	switch v {
	case ChannelsMessageTypeResponse:
		return "response"
	case ChannelsMessageTypeReject:
		return "reject"
	case ChannelsMessageTypeMerkleProof:
		return "merkle_proof"
	default:
		return ""
	}
}

func (v *RejectReason) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for RejectReason : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v RejectReason) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v RejectReason) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown RejectReason value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *RejectReason) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *RejectReason) SetString(s string) error {
	switch s {
	case "in_use":
		*v = RejectReasonChannelInUse
	case "unwanted":
		*v = RejectReasonUnwanted
	default:
		*v = RejectReasonInvalid
		return fmt.Errorf("Unknown RejectReason value \"%s\"", s)
	}

	return nil
}

func (v RejectReason) String() string {
	switch v {
	case RejectReasonChannelInUse:
		return "in_use"
	case RejectReasonUnwanted:
		return "unwanted"
	default:
		return ""
	}
}
