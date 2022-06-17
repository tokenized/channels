package channels

import (
	"bytes"
	"fmt"
	"sync"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

var (
	ErrNotChannels         = errors.New("Not Channels")
	ErrInvalidMessage      = errors.New("Invalid Message")
	ErrUnsupportedVersion  = errors.New("Unsupported Version")
	ErrUnsupportedProtocol = errors.New("Unsupported Protocol")
	ErrNotSupported        = errors.New("Not Supported")
)

// Message is implemented by all channels protocol message types. It is used to identify the
// specific channels protocol for the message.
// ChannelsMessages must also implement either Writer or Wrapper though this can't be enforced at
// compile time.
type Message interface {
	ProtocolID() envelope.ProtocolID
}

// Writer is implemented by Channels messages that can't wrap other message types. It returns the
// envelope protocol IDs and payload.
type Writer interface {
	Message
	Write() (envelope.Data, error)
}

// Wrapper is implemented by Channels messages that can wrap other message types. For instance a
// signature message can be wrapped around a another message. It returns the envelope protocol IDs
// and payload.
type Wrapper interface {
	Message
	Wrap(envelope.Data) (envelope.Data, error)
}

type PeerChannel struct {
	BaseURL    string `bsor:"1" json:"base_url"`
	ID         string `bsor:"2" json:"id"`
	ReadToken  string `bsor:"3" json:"read_token,omitempty"`
	WriteToken string `bsor:"4" json:"write_token,omitempty"`
}

type PeerChannels []PeerChannel

type WrappedMessage struct {
	Signature *Signature
	Response  *Response
	MessageID *MessageID
	Message   Message
}

type Protocol interface {
	ProtocolID() envelope.ProtocolID
	Parse(payload envelope.Data) (Message, error)
	ResponseCodeToString(code uint32) string // convert Response.Code to a string
}

type Protocols struct {
	Protocols []Protocol

	lock sync.RWMutex
}

func NewProtocols(protocols ...Protocol) *Protocols {
	result := &Protocols{}

	for _, protocol := range protocols {
		result.Protocols = append(result.Protocols, protocol)
	}

	return result
}

func (ps *Protocols) AddProtocols(protocols ...Protocol) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, protocol := range protocols {
		ps.Protocols = append(ps.Protocols, protocol)
	}
}

func (ps *Protocols) GetProtocol(protocolID envelope.ProtocolID) Protocol {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	for _, protocol := range ps.Protocols {
		if bytes.Equal(protocol.ProtocolID(), protocolID) {
			return protocol
		}
	}

	return nil
}

func (ps *Protocols) ResponseCodeToString(protocolID envelope.ProtocolID, code uint32) string {
	if code == 0 {
		return protocolID.String() + ":parse"
	}

	ps.lock.RLock()
	defer ps.lock.RUnlock()

	for _, protocol := range ps.Protocols {
		if bytes.Equal(protocol.ProtocolID(), protocolID) {
			return protocolID.String() + ":" + protocol.ResponseCodeToString(code)
		}
	}

	return protocolID.String() + fmt.Sprintf(":unknown(%d)", code)
}

// Wrap wraps a message with a response id (if specified), adds the message id, signs it, and
// serializes it.
func Wrap(msg Writer, key bitcoin.Key, hash bitcoin.Hash32, messageID uint64,
	responseID *uint64) (bitcoin.Script, error) {

	payload, err := msg.Write()
	if err != nil {
		return nil, errors.Wrap(err, "write")
	}

	// Don't put two responses in the message.
	if _, ok := msg.(*Response); !ok && responseID != nil {
		payload, err = WrapResponseID(payload, *responseID)
		if err != nil {
			return nil, errors.Wrap(err, "response")
		}
	}

	payload, err = WrapMessageID(payload, messageID)
	if err != nil {
		return nil, errors.Wrap(err, "message id")
	}

	payload, err = WrapSignature(payload, key, &hash, false)
	if err != nil {
		return nil, errors.Wrap(err, "sign")
	}

	return envelopeV1.Wrap(payload).Script()
}

// WrapWithResponse wraps a message with the specified response, adds the message id, signs it, and
// serializes it.
func WrapWithResponse(msg Writer, response *Response, key bitcoin.Key, hash bitcoin.Hash32,
	messageID uint64) (bitcoin.Script, error) {

	payload, err := msg.Write()
	if err != nil {
		return nil, errors.Wrap(err, "write")
	}

	payload, err = response.Wrap(payload)
	if err != nil {
		return nil, errors.Wrap(err, "response")
	}

	payload, err = WrapMessageID(payload, messageID)
	if err != nil {
		return nil, errors.Wrap(err, "message id")
	}

	payload, err = WrapSignature(payload, key, &hash, false)
	if err != nil {
		return nil, errors.Wrap(err, "sign")
	}

	return envelopeV1.Wrap(payload).Script()
}

func (ps *Protocols) Unwrap(script []byte) (*WrappedMessage, error) {
	payload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		return nil, errors.Wrap(err, "envelope")
	}

	result := &WrappedMessage{}
	result.Signature, payload, err = ParseSigned(payload)
	if err != nil {
		return nil, errors.Wrap(err, "sign")
	}

	result.MessageID, payload, err = ParseMessageID(payload)
	if err != nil {
		return nil, errors.Wrap(err, "message id")
	}

	result.Response, payload, err = ParseResponse(payload)
	if err != nil {
		return nil, errors.Wrap(err, "response")
	}

	if len(payload.ProtocolIDs) == 0 {
		return result, nil
	}

	if len(payload.ProtocolIDs) > 1 {
		return nil, errors.Wrap(ErrNotSupported, "more than one data protocol")
	}

	protocol := ps.GetProtocol(payload.ProtocolIDs[0])
	if protocol == nil {
		return nil, errors.Wrap(ErrUnsupportedProtocol, payload.ProtocolIDs[0].String())
	}

	msg, err := protocol.Parse(payload)
	if err != nil {
		return nil, errors.Wrap(err, "parse")
	}
	result.Message = msg

	return result, nil
}
