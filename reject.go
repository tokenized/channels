package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"

	"github.com/pkg/errors"
)

const (
	RejectVersion = uint8(0)

	RejectReasonUnspecified = RejectReason(0)

	// RejectReasonInvalid means something in a sub protocol was invalid. The protocol ID with the
	// issue and a protocol specific code should be provided.
	RejectReasonInvalid = RejectReason(1)

	// RejectReasonUnsupportedProtocol means the message received used a protocol not supported by
	// this software.
	RejectReasonUnsupportedProtocol = RejectReason(2)

	// RejectReasonUnwanted means the request message received was valid, but the recipient doesn't
	// want to accept it.
	RejectReasonUnwanted = RejectReason(3)

	// RejectReasonNeedPayment means that a payment request was previously exchanged and not yet
	// fulfilled. Until that is fulfilled or renegotiated further requests will be rejected.
	RejectReasonNeedPayment = RejectReason(4)

	// RejectReasonChannelInUse means the peer channel the request was received on is already in use
	// for another purpose.
	RejectReasonChannelInUse = RejectReason(5)

	// RejectReasonSystemIssue means there was a systems issue and it was important to respond, but
	// a successful response was not possible.
	RejectReasonSystemIssue = RejectReason(6)
)

var (
	ProtocolIDReject = envelope.ProtocolID("RJ") // Protocol ID for channel reject messages
)

type RejectReason uint8

// Reject is used to reject a previous message.
type Reject struct {
	MessageID        uint64              `bsor:"1" json:"message_id"`
	Reason           RejectReason        `bsor:"2" json:"reason"`
	RejectProtocolID envelope.ProtocolID `bsor:"3" json:"protocol_id"`
	Code             uint32              `bsor:"4" json:"code"` // Sub protocol specific codes
	Note             string              `bsor:"5" json:"note"`
}

func (r Reject) Error() string {
	if len(r.Note) > 0 {
		return fmt.Sprintf("%s: %s", r.CodeToString(), r.Note)
	}
	return r.CodeToString()
}

func (*Reject) ProtocolID() envelope.ProtocolID {
	return ProtocolIDReject
}

func (r *Reject) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(RejectVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(r)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDReject}, payload}, nil
}

func (r Reject) CodeToString() string {
	if bytes.Equal(r.RejectProtocolID, ProtocolIDSignedMessages) {
		return "signed:" + SignedRejectCodeToString(r.Code)
	}

	if bytes.Equal(r.RejectProtocolID, ProtocolIDMerkleProof) {
		return "merkle proof:" + MerkleProofRejectCodeToString(r.Code)
	}

	if bytes.Equal(r.RejectProtocolID, ProtocolIDRelationships) {
		return "relationships:" + RelationshipsRejectCodeToString(r.Code)
	}

	if bytes.Equal(r.RejectProtocolID, ProtocolIDInvoices) {
		return "invoices:" + InvoicesRejectCodeToString(r.Code)
	}

	if bytes.Equal(r.RejectProtocolID, ProtocolIDResponse) {
		return "response:" + ResponseRejectCodeToString(r.Code)
	}

	if r.Code == 0 {
		return r.RejectProtocolID.String() + ":parse"
	}

	return r.RejectProtocolID.String() + ":unknown"
}

func ParseReject(payload envelope.Data) (*Reject, error) {
	if len(payload.ProtocolIDs) == 0 {
		return nil, nil
	}

	if !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDReject) {
		return nil, nil
	}

	if len(payload.ProtocolIDs) != 1 {
		return nil, errors.Wrapf(ErrInvalidMessage, "rejects can't wrap")
	}

	if len(payload.Payload) == 0 {
		return nil, errors.Wrapf(ErrInvalidMessage, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(ErrUnsupportedVersion, fmt.Sprintf("reject: %d", version))
	}

	result := &Reject{}
	if _, err := bsor.Unmarshal(payload.Payload[1:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
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
	case "unsupported_protocol":
		*v = RejectReasonUnsupportedProtocol
	case "invalid":
		*v = RejectReasonInvalid
	case "unwanted":
		*v = RejectReasonUnwanted
	case "need_payment":
		*v = RejectReasonNeedPayment
	case "in_use":
		*v = RejectReasonChannelInUse
	case "system_issue":
		*v = RejectReasonSystemIssue
	case "unspecified":
		*v = RejectReasonUnspecified
	default:
		*v = RejectReasonUnspecified
		return fmt.Errorf("Unknown RejectReason value \"%s\"", s)
	}

	return nil
}

func (v RejectReason) String() string {
	switch v {
	case RejectReasonUnsupportedProtocol:
		return "unsupported_protocol"
	case RejectReasonInvalid:
		return "invalid"
	case RejectReasonUnwanted:
		return "unwanted"
	case RejectReasonNeedPayment:
		return "need_payment"
	case RejectReasonChannelInUse:
		return "in_use"
	case RejectReasonSystemIssue:
		return "system_issue"
	case RejectReasonUnspecified:
		return "unspecified"
	default:
		return ""
	}
}
