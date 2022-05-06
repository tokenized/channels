package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/merkle_proof"

	"github.com/pkg/errors"
)

const (
	MerkleProofVersion = uint8(0)
)

var (
	ProtocolIDMerkleProof = envelope.ProtocolID("MP") // Protocol ID for merkle proofs

	ErrInvalidMerkleProof = errors.New("Invalid MerkleProof")
)

type MerkleProof merkle_proof.MerkleProof

func (*MerkleProof) ProtocolID() envelope.ProtocolID {
	return ProtocolIDMerkleProof
}

func MerkleProofRejectCodeToString(code uint32) string {
	switch code {
	default:
		return "parse_error"
	}
}

func WriteMerkleProof(message *merkle_proof.MerkleProof) (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(MerkleProofVersion))}

	// Message
	b, err := message.MarshalBinary()
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, bitcoin.NewPushDataScriptItem(b))

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDMerkleProof}, payload}, nil
}

func ParseMerkleProof(payload envelope.Data) (*MerkleProof, error) {

	if len(payload.ProtocolIDs) == 0 ||
		!bytes.Equal(payload.ProtocolIDs[0], ProtocolIDMerkleProof) {
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
		return nil, errors.Wrap(ErrUnsupportedVersion, fmt.Sprintf("merkle proof: %d", version))
	}

	if payload.Payload[1].Type != bitcoin.ScriptItemTypePushData {
		return nil, errors.Wrap(ErrInvalidMerkleProof, "not push data")
	}

	var merkleProof merkle_proof.MerkleProof
	if err := merkleProof.UnmarshalBinary(payload.Payload[1].Data); err != nil {
		return nil, errors.Wrap(ErrInvalidMerkleProof, err.Error())
	}
	result := MerkleProof(merkleProof)

	return &result, nil
}
