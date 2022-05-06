package channels

import (
	"bytes"
	"crypto/sha256"
	"fmt"

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
)

// ChannelsMessage is implemented by all channels protocol message types. It is used to identify the
// specific channels protocol for the message.
// ChannelsMessages must also implement either Writer or Wrapper though this can't be enforced at
// compile time.
type ChannelsMessage interface {
	ProtocolID() envelope.ProtocolID
}

// Writer is implemented by Channels messages that can't wrap other message types. It returns the
// envelope protocol IDs and payload.
type Writer interface {
	Write() (envelope.Data, error)
}

// Wrapper is implemented by Channels messages that can wrap other message types. For instance a
// signature message can be wrapped around a another message. It returns the envelope protocol IDs
// and payload.
type Wrapper interface {
	Wrap(envelope.Data) (envelope.Data, error)
}

type PeerChannel struct {
	BaseURL    string `bsor:"1" json:"base_url"`
	ID         string `bsor:"2" json:"id"`
	WriteToken string `bsor:"3" json:"write_token"`
}

type PeerChannels []PeerChannel

func MessageHash(payload envelope.Data) bitcoin.Hash32 {
	scriptItems := envelopeV1.Wrap(payload)
	script, _ := scriptItems.Script()
	return bitcoin.Hash32(sha256.Sum256(script))
}

type WrappedMessage struct {
	Signature *Signature
	Response  *Response
	Message   ChannelsMessage
}

func Wrap(msg Writer, key bitcoin.Key, hash bitcoin.Hash32,
	responseHash *bitcoin.Hash32) (bitcoin.Script, error) {

	payload, err := msg.Write()
	if err != nil {
		return nil, errors.Wrap(err, "write")
	}

	if responseHash != nil {
		response := &Response{
			MessageHash: *responseHash,
		}
		payload, err = response.Wrap(payload)
		if err != nil {
			return nil, errors.Wrap(err, "response")
		}
	}

	signature, err := Sign(payload, key, &hash, false)
	if err != nil {
		return nil, errors.Wrap(err, "sign")
	}

	payload, err = signature.Wrap(payload)
	if err != nil {
		return nil, errors.Wrap(err, "sign wrap")
	}

	return envelopeV1.Wrap(payload).Script()
}

func Unwrap(script []byte) (*WrappedMessage, error) {
	payload, err := envelopeV1.Parse(bytes.NewReader(script))
	if err != nil {
		return nil, errors.Wrap(err, "envelope")
	}

	result := &WrappedMessage{}
	result.Signature, payload, err = ParseSigned(payload)
	if err != nil {
		return nil, errors.Wrap(err, "sign")
	}

	result.Response, payload, err = ParseResponse(payload)
	if err != nil {
		return nil, errors.Wrap(err, "response")
	}

	result.Message, err = parse(payload)
	if err != nil {
		return nil, errors.Wrap(err, "parse")
	}

	return result, nil
}

func parse(payload envelope.Data) (ChannelsMessage, error) {
	if len(payload.ProtocolIDs) == 0 {
		return nil, errors.New("Message empty")
	}

	if bytes.Equal(payload.ProtocolIDs[0], ProtocolIDMerkleProof) {
		return ParseMerkleProof(payload)
	}

	if bytes.Equal(payload.ProtocolIDs[0], ProtocolIDRelationships) {
		return ParseRelationship(payload)
	}

	if bytes.Equal(payload.ProtocolIDs[0], ProtocolIDInvoices) {
		return ParseInvoice(payload)
	}

	return nil, errors.Wrap(ErrUnsupportedProtocol, fmt.Sprintf("%x", payload.ProtocolIDs[0]))
}
