package expanded_tx

import (
	"bytes"
	"fmt"

	"github.com/tokenized/channels"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/expanded_tx"

	"github.com/pkg/errors"
)

const (
	Version = uint8(0)

	// ResponseCodeTxRejected means the tx was rejected by the Bitcoin network.
	ResponseCodeTxRejected = uint32(1)

	// ResponseCodeMissingInputs means that neither spent outputs or ancestors were provided for the
	// inputs in the tx. Some applications may need this data to properly process a transaction.
	ResponseCodeMissingInputs = uint32(2)

	// ResponseCodeMissingAncestors means that ancestors were not provided for the inputs in the tx.
	// Spent outputs may have been provided, but full ancestor transactions are required. Some
	// applications may need this data to properly process a transaction.
	ResponseCodeMissingAncestors = uint32(3)

	// ResponseCodeTxFeeTooLow means the tx miner fee is too low.
	ResponseCodeTxFeeTooLow = uint32(4)
)

var (
	ProtocolID = envelope.ProtocolID("ETX") // Protocol ID for expanded tx
)

type Protocol struct{}

// ExpandedTx is a channels protocol message that contains an expanded tx. It can't be embedded in
// the base expanded_tx package because it is a circular dependency with the channels.Message.
type ExpandedTxMessage expanded_tx.ExpandedTx

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

func (*ExpandedTxMessage) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *ExpandedTxMessage) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message
	msgScriptItems, err := bsor.Marshal(m)
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
		return nil, payload, errors.Wrapf(channels.ErrInvalidMessage, "expanded tx messages can't wrap")
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
			fmt.Sprintf("expanded tx %d", version))
	}

	result := &ExpandedTxMessage{}
	payloads, err := bsor.Unmarshal(payload.Payload[1:], result)
	if err != nil {
		return nil, payload, errors.Wrap(err, "unmarshal")
	}
	payload.Payload = payloads

	return result, payload, nil
}

func ResponseCodeToString(code uint32) string {
	switch code {
	case ResponseCodeTxRejected:
		return "tx_rejected"
	case ResponseCodeMissingInputs:
		return "missing_inputs"
	case ResponseCodeMissingAncestors:
		return "missing_ancestors"
	case ResponseCodeTxFeeTooLow:
		return "tx_fee_too_low"
	default:
		return "parse_error"
	}
}
