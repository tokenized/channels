package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/fees"

	"github.com/pkg/errors"
)

const (
	FeeRequirementsVersion = uint8(0)
)

var (
	ProtocolIDFeeRequirements = envelope.ProtocolID("FEES") // Protocol ID for channel fee requirements
)

type FeeRequirementsProtocol struct{}

// FeeRequirementsMessage is a channels protocol message that contains fee requirements. It can't be
// embedded in the base fees package because it is a circular dependency with the channels.Message.
type FeeRequirementsMessage fees.FeeRequirements

func NewFeeRequirementsProtocol() *FeeRequirementsProtocol {
	return &FeeRequirementsProtocol{}
}

func (*FeeRequirementsProtocol) ProtocolID() envelope.ProtocolID {
	return ProtocolIDFeeRequirements
}

func (*FeeRequirementsProtocol) Parse(payload envelope.Data) (Message, envelope.Data, error) {
	return ParseFeeRequirements(payload)
}

func (*FeeRequirementsProtocol) ResponseCodeToString(code uint32) string {
	return FeeRequirementsResponseCodeToString(code)
}

func (m *FeeRequirementsMessage) GetFeeRequirements() fees.FeeRequirements {
	return fees.FeeRequirements(*m)
}

func NewFeeRequirementsMessage(fr fees.FeeRequirements) *FeeRequirementsMessage {
	cfr := FeeRequirementsMessage(fr)
	return &cfr
}

func (*FeeRequirementsMessage) IsWrapperType() {}

func (*FeeRequirementsMessage) ProtocolID() envelope.ProtocolID {
	return ProtocolIDFeeRequirements
}

func (r *FeeRequirementsMessage) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(FeeRequirementsVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDFeeRequirements}, payload}, nil
}

func (r *FeeRequirementsMessage) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(FeeRequirementsVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	scriptItems = append(scriptItems, msgScriptItems...)

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDFeeRequirements}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func ParseFeeRequirements(payload envelope.Data) (*FeeRequirementsMessage, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 ||
		!bytes.Equal(payload.ProtocolIDs[0], ProtocolIDFeeRequirements) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 2 {
		return nil, payload, errors.Wrapf(ErrInvalidMessage,
			"not enough fee requirements push ops: %d", len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion,
			fmt.Sprintf("fee requirements: %d", version))
	}

	result := &FeeRequirementsMessage{}
	payloads, err := bsor.Unmarshal(payload.Payload[1:], result)
	if err != nil {
		return nil, payload, errors.Wrap(err, "unmarshal")
	}
	payload.Payload = payloads

	return result, payload, nil
}

func FeeRequirementsResponseCodeToString(code uint32) string {
	switch code {
	default:
		return "parse_error"
	}
}
