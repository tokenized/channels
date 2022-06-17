package channels

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"

	"github.com/pkg/errors"
)

const (
	SignedMessagesVersion = uint8(0)

	// SignedStatusSignatureRequired is a code specific to the signature protocol that is placed
	// in a Reject message to signify that a message is considered invalid if it is not signed.
	SignedStatusSignatureRequired = uint32(1)

	// SignedStatusInvalidSignature is a code specific to the signature protocol that is placed
	// in a Reject message to signify that a message has an invalid signature. Either the wrong
	// key was used or the signature is just invalid.
	SignedStatusInvalidSignature = uint32(2)

	// SignedStatusWrongPublicKey is a code specific to the signature protocol that is placed
	// in a Reject message to signify that a message includes the wrong public key for context.
	SignedStatusWrongPublicKey = uint32(3)
)

var (
	ProtocolIDSignedMessages = envelope.ProtocolID("S") // Protocol ID for signed messages

	ErrPublicKeyMissing = errors.New("Public Key Missing")
	ErrHashMissing      = errors.New("Hash Missing")
	ErrInvalidSignature = errors.New("Invalid Signature")
)

// SignedMessage is a message signed by a key.
// First push op is public key, or OP_FALSE if no key is provided.
// Second push op is signature of remaining protocol ids and push ops.
type Signature struct {
	Signature      bitcoin.Signature  `bsor:"1" json:"signature"`
	PublicKey      *bitcoin.PublicKey `bsor:"2" json:"public_key"`
	DerivationHash *bitcoin.Hash32    `bsor:"3" json:"derivation_hash"`
	hash           *bitcoin.Hash32
}

func (*Signature) ProtocolID() envelope.ProtocolID {
	return ProtocolIDSignedMessages
}

func (s *Signature) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(SignedMessagesVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(s)
	if err != nil {
		return payload, errors.Wrap(err, "marshal")
	}
	scriptItems = append(scriptItems, msgScriptItems...)

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDSignedMessages},
		payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

// Sign adds the SignedMessage protocol to the provided bitcoin script with the signature of the
// script and the key if specified.
func Sign(payload envelope.Data, key bitcoin.Key, derivationHash *bitcoin.Hash32,
	includeKey bool) (*Signature, error) {

	hasher := sha256.New()
	for _, protocolID := range payload.ProtocolIDs {
		hasher.Write(protocolID)
	}

	if err := payload.Payload.Write(hasher); err != nil {
		return nil, errors.Wrap(err, "script")
	}

	hash, err := bitcoin.NewHash32(hasher.Sum(nil))
	if err != nil {
		return nil, errors.Wrap(err, "new hash")
	}

	var signature bitcoin.Signature
	if derivationHash != nil {
		derivedKey, err := key.AddHash(*derivationHash)
		if err != nil {
			return nil, errors.Wrap(err, "derive key")
		}

		signature, err = derivedKey.Sign(*hash)
		if err != nil {
			return nil, errors.Wrap(err, "sign")
		}
	} else {
		signature, err = key.Sign(*hash)
		if err != nil {
			return nil, errors.Wrap(err, "sign")
		}
	}

	message := &Signature{
		Signature:      signature,
		DerivationHash: derivationHash,
	}

	if includeKey {
		publicKey := key.PublicKey()
		message.PublicKey = &publicKey
	}

	return message, nil
}

// WrapSignature signs the payload and wraps the payload with the signature and returns the new
// payload containing the signature.
func WrapSignature(payload envelope.Data, key bitcoin.Key, derivationHash *bitcoin.Hash32,
	includeKey bool) (envelope.Data, error) {

	signature, err := Sign(payload, key, derivationHash, includeKey)
	if err != nil {
		return payload, errors.Wrap(err, "sign")
	}

	return signature.Wrap(payload)
}

// ParseSigned parses the signature and public key (if provided).
func ParseSigned(payload envelope.Data) (*Signature, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 ||
		!bytes.Equal(payload.ProtocolIDs[0], ProtocolIDSignedMessages) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 2 {
		return nil, payload, errors.Wrapf(ErrInvalidMessage, "not enough signature push ops: %d",
			len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion, fmt.Sprintf("signed: %d", version))
	}
	payload.Payload = payload.Payload[1:]

	signature := &Signature{}
	payload.Payload, err = bsor.Unmarshal(payload.Payload, signature)
	if err != nil {
		return nil, payload, errors.Wrap(err, "unmarshal")
	}

	hasher := sha256.New()
	for _, protocolID := range payload.ProtocolIDs {
		hasher.Write(protocolID)
	}
	if err := payload.Payload.Write(hasher); err != nil {
		return nil, payload, errors.Wrap(err, "hash")
	}

	hash, err := bitcoin.NewHash32(hasher.Sum(nil))
	if err != nil {
		return nil, payload, errors.Wrap(err, "new hash")
	}
	signature.hash = hash

	return signature, payload, nil
}

// GetPublicKey calculates the public key if there is a derivation hash or just returns the included
// public key.
func (m Signature) GetPublicKey() (*bitcoin.PublicKey, error) {
	if m.PublicKey == nil {
		return nil, ErrPublicKeyMissing
	}

	if m.DerivationHash == nil {
		return m.PublicKey, nil
	}

	publicKey, err := m.PublicKey.AddHash(*m.DerivationHash)
	if err != nil {
		return nil, errors.Wrap(err, "derive key")
	}

	return &publicKey, nil
}

// SetPublicKey sets the base public key on the message. To be used when the public key isn't
// provided in the message but is known from context.
func (m *Signature) SetPublicKey(publicKey *bitcoin.PublicKey) {
	m.PublicKey = publicKey
}

func (m Signature) Verify() error {
	if m.PublicKey == nil {
		return ErrPublicKeyMissing
	}
	if m.hash == nil {
		return ErrHashMissing
	}

	if m.DerivationHash == nil {
		if !m.Signature.Verify(*m.hash, *m.PublicKey) {
			return ErrInvalidSignature
		}

		return nil
	}

	publicKey, err := m.PublicKey.AddHash(*m.DerivationHash)
	if err != nil {
		return errors.Wrap(err, "derive key")
	}

	if !m.Signature.Verify(*m.hash, publicKey) {
		return ErrInvalidSignature
	}

	return nil
}

func SignedStatusToString(code uint32) string {
	switch code {
	case SignedStatusSignatureRequired:
		return "signature_required"
	case SignedStatusInvalidSignature:
		return "invalid_signature"
	case SignedStatusWrongPublicKey:
		return "wrong_public_key"
	default:
		return "parse_error"
	}
}
