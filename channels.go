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

	ErrRemainingProtocols        = errors.New("Remaining Protocols")
	ErrParseDidntConsumeProtocol = errors.New("Parse Didn't Consume Protocol")
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
	Wrap(payload envelope.Data) (envelope.Data, error)
}

type PeerChannel struct {
	BaseURL    string `bsor:"1" json:"base_url"`
	ID         string `bsor:"2" json:"id"`
	ReadToken  string `bsor:"3" json:"read_token,omitempty"`
	WriteToken string `bsor:"4" json:"write_token,omitempty"`
}

type PeerChannels []PeerChannel

type Protocol interface {
	ProtocolID() envelope.ProtocolID
	Parse(payload envelope.Data) (Message, envelope.Data, error)
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

func (ps *Protocols) Parse(script bitcoin.Script) (Message, []Wrapper, error) {
	payload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		return nil, nil, errors.Wrap(err, "envelope")
	}

	var wrappers []Wrapper
	for {
		protocol := ps.GetProtocol(payload.ProtocolIDs[0])
		if protocol == nil {
			return nil, wrappers, errors.Wrap(ErrUnsupportedProtocol,
				payload.ProtocolIDs[0].String())
		}

		msg, newPayload, err := protocol.Parse(payload)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "parse: %s", protocol.ProtocolID())
		}

		if len(newPayload.ProtocolIDs) == 0 {
			return msg, wrappers, nil
		}

		if wrapper, ok := msg.(Wrapper); ok {
			wrappers = append(wrappers, wrapper)
		} else {
			return nil, nil, errors.Wrapf(ErrRemainingProtocols, "%s", newPayload.ProtocolIDs)
		}

		if len(payload.ProtocolIDs) == len(newPayload.ProtocolIDs) {
			return nil, nil, errors.Wrapf(ErrParseDidntConsumeProtocol, "%s", newPayload.ProtocolIDs)
		}

		payload = newPayload
	}
}

func WrapEnvelope(msg Writer, wrappers ...Wrapper) (envelope.Data, error) {
	var payload envelope.Data
	var err error

	if msg != nil {
		payload, err = msg.Write()
		if err != nil {
			return payload, errors.Wrap(err, "write")
		}
	}

	for _, wrapper := range wrappers {
		if wrapper == nil {
			continue
		}

		payload, err = wrapper.Wrap(payload)
		if err != nil {
			return payload, errors.Wrap(err, "reply to")
		}
	}

	return payload, nil
}

func Wrap(msg Writer, wrappers ...Wrapper) (bitcoin.Script, error) {
	data, err := WrapEnvelope(msg, wrappers...)
	if err != nil {
		return nil, errors.Wrap(err, "envelope")
	}

	return envelopeV1.Wrap(data).Script()
}
