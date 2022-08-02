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
	ChannelsVersion = uint8(0)
)

var (
	ChannelsProtocolID = envelope.ProtocolID("ETX") // Protocol ID for teller
)

type ChannelsProtocol struct{}

// ExpandedTx is a channels protocol message that contains an expanded tx. It can't be embedded in
// the base expanded_tx package because it is a circular dependency with the channels.Message.
type ExpandedTxMessage expanded_tx.ExpandedTx

func NewProtocol() *ChannelsProtocol {
	return &ChannelsProtocol{}
}

func (*ChannelsProtocol) ProtocolID() envelope.ProtocolID {
	return ChannelsProtocolID
}

func (*ChannelsProtocol) Parse(payload envelope.Data) (channels.Message, error) {
	return Parse(payload)
}

func (*ChannelsProtocol) ResponseCodeToString(code uint32) string {
	switch code {
	default:
		return "parse_error"
	}
}

func (*ExpandedTxMessage) ProtocolID() envelope.ProtocolID {
	return ChannelsProtocolID
}

func (m *ExpandedTxMessage) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(ChannelsVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ChannelsProtocolID}, payload}, nil
}

func Parse(payload envelope.Data) (channels.Message, error) {
	if len(payload.ProtocolIDs) == 0 {
		return nil, nil
	}

	if !bytes.Equal(payload.ProtocolIDs[0], ChannelsProtocolID) {
		return nil, nil
	}

	if len(payload.ProtocolIDs) != 1 {
		return nil, errors.Wrapf(channels.ErrInvalidMessage, "expanded tx messages can't wrap")
	}

	if len(payload.Payload) == 0 {
		return nil, errors.Wrapf(channels.ErrInvalidMessage, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(channels.ErrUnsupportedVersion,
			fmt.Sprintf("expanded tx %d", version))
	}

	result := &ExpandedTxMessage{}
	if _, err := bsor.Unmarshal(payload.Payload[1:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}
