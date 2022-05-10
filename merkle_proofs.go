package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
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

func MerkleProofRejectCodeToString(code uint32) string {
	switch code {
	default:
		return "parse_error"
	}
}

type MerkleProof struct {
	MerkleProof *merkle_proof.MerkleProof `bsor:"1" json:"merkle_proof"`
}

func (*MerkleProof) ProtocolID() envelope.ProtocolID {
	return ProtocolIDMerkleProof
}

func (m *MerkleProof) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(MerkleProofVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

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

	result := &MerkleProof{}
	if _, err := bsor.Unmarshal(payload.Payload[1:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}
