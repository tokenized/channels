package merkle_proofs

import (
	"bytes"
	"fmt"

	"github.com/tokenized/channels"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/merkle_proof"

	"github.com/pkg/errors"
)

const (
	Version = uint8(0)
)

var (
	ProtocolID = envelope.ProtocolID("MP") // Protocol ID for merkle proofs

	ErrInvalidMerkleProof = errors.New("Invalid MerkleProof")
)

type Protocol struct{}

func NewProtocol() *Protocol {
	return &Protocol{}
}

func (*Protocol) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (*Protocol) Parse(payload envelope.Data) (channels.Message, error) {
	return Parse(payload)
}

func (*Protocol) ResponseCodeToString(code uint32) string {
	return ResponseCodeToString(code)
}

type MerkleProof struct {
	MerkleProof *merkle_proof.MerkleProof `bsor:"1" json:"merkle_proof"`
}

func (*MerkleProof) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *MerkleProof) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message
	b, err := m.MerkleProof.MarshalBinary()
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal binary")
	}
	payload = append(payload, bitcoin.NewPushDataScriptItem(b))

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

func Parse(payload envelope.Data) (*MerkleProof, error) {
	if len(payload.ProtocolIDs) == 0 ||
		!bytes.Equal(payload.ProtocolIDs[0], ProtocolID) {
		return nil, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 2 {
		return nil, errors.Wrapf(ErrInvalidMerkleProof, "not enough merkle proof push ops: %d",
			len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(channels.ErrUnsupportedVersion,
			fmt.Sprintf("merkle proof: %d", version))
	}

	result := &MerkleProof{}
	if payload.Payload[1].Type != bitcoin.ScriptItemTypePushData {
		return nil, errors.Wrapf(ErrInvalidMerkleProof, "not push data")
	}

	result.MerkleProof = &merkle_proof.MerkleProof{}
	if err := result.MerkleProof.UnmarshalBinary(payload.Payload[1].Data); err != nil {
		return nil, errors.Wrap(err, "unmarshal binary")
	}

	return result, nil
}

func ResponseCodeToString(code uint32) string {
	switch code {
	default:
		return "parse_error"
	}
}
