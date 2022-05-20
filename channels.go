package channels

import (
	"bytes"
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

type WrappedMessage struct {
	Signature *Signature
	Response  *Response
	MessageID *MessageID
	Message   Message
}

func Wrap(msg Writer, key bitcoin.Key, hash bitcoin.Hash32, messageID uint64,
	responseID *uint64) (bitcoin.Script, error) {

	payload, err := msg.Write()
	if err != nil {
		return nil, errors.Wrap(err, "write")
	}

	mID := &MessageID{
		MessageID: messageID,
	}
	payload, err = mID.Wrap(payload)
	if err != nil {
		return nil, errors.Wrap(err, "message id")
	}

	if responseID != nil {
		response := &Response{
			MessageID: *responseID,
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

	result.MessageID, payload, err = ParseMessageID(payload)
	if err != nil {
		return nil, errors.Wrap(err, "message id")
	}

	result.Message, err = parse(payload)
	if err != nil {
		return nil, errors.Wrap(err, "parse")
	}

	return result, nil
}

func parse(payload envelope.Data) (Message, error) {
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

	if bytes.Equal(payload.ProtocolIDs[0], ProtocolIDReject) {
		return ParseReject(payload)
	}

	return nil, errors.Wrap(ErrUnsupportedProtocol, fmt.Sprintf("0x%x", payload.ProtocolIDs[0]))
}
