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
	// SignedRejectCodeSignatureRequired is a code specific to the signature protocol that is placed
	// in a Reject message to signify that a message is considered invalid if it is not signed.
	SignedRejectCodeSignatureRequired = uint32(1)

	// SignedRejectCodeInvalidSignature is a code specific to the signature protocol that is placed
	// in a Reject message to signify that a message has an invalid signature. Either the wrong
	// key was used or the signature is just invalid.
	SignedRejectCodeInvalidSignature = uint32(2)

	// SignedRejectCodeWrongPublicKey is a code specific to the signature protocol that is placed
	// in a Reject message to signify that a message includes the wrong public key for context.
	SignedRejectCodeWrongPublicKey = uint32(3)
)

var (
	ProtocolIDSignedMessages = envelope.ProtocolID("S") // Protocol ID for signed messages
	SignedMessagesVersion    = uint8(0)

	ErrNotSignedMessage                 = errors.New("Not Signed Message")
	ErrUnsupportedSignedMessagesVersion = errors.New("Unsupported Signed Messages Version")
	ErrPublicKeyMissing                 = errors.New("Public Key Missing")
	ErrHashMissing                      = errors.New("Hash Missing")
	ErrInvalidSignature                 = errors.New("Invalid Signature")
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

// Sign adds the SignedMessage protocol to the provided bitcoin script with the signature of the
// script and the key is specified.
func Sign(protocolIDs envelope.ProtocolIDs, payload bitcoin.ScriptItems, key bitcoin.Key,
	derivationHash *bitcoin.Hash32,
	includeKey bool) (envelope.ProtocolIDs, bitcoin.ScriptItems, error) {

	hasher := sha256.New()
	for _, protocolID := range protocolIDs {
		hasher.Write(protocolID)
	}

	if err := payload.Write(hasher); err != nil {
		return nil, nil, errors.Wrap(err, "script")
	}

	hash, err := bitcoin.NewHash32(hasher.Sum(nil))
	if err != nil {
		return nil, nil, errors.Wrap(err, "new hash")
	}

	var signature bitcoin.Signature
	if derivationHash != nil {
		derivedKey, err := key.AddHash(*derivationHash)
		if err != nil {
			return nil, nil, errors.Wrap(err, "derive key")
		}

		signature, err = derivedKey.Sign(*hash)
		if err != nil {
			return nil, nil, errors.Wrap(err, "sign")
		}
	} else {
		signature, err = key.Sign(*hash)
		if err != nil {
			return nil, nil, errors.Wrap(err, "sign")
		}
	}

	// Version
	result := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(SignedMessagesVersion))}

	message := &Signature{
		Signature:      signature,
		DerivationHash: derivationHash,
	}

	if includeKey {
		publicKey := key.PublicKey()
		message.PublicKey = &publicKey
	}

	signatureScriptItems, err := bsor.Marshal(message)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal")
	}
	result = append(result, signatureScriptItems...)

	return append(envelope.ProtocolIDs{ProtocolIDSignedMessages}, protocolIDs...),
		append(result, payload...), nil
}

// ParseSigned parses the signature and public key (if provided).
func ParseSigned(protocolIDs envelope.ProtocolIDs,
	payload bitcoin.ScriptItems) (*Signature, envelope.ProtocolIDs, bitcoin.ScriptItems, error) {

	if len(protocolIDs) == 0 || !bytes.Equal(protocolIDs[0], ProtocolIDSignedMessages) {
		return nil, protocolIDs, payload, nil
	}
	protocolIDs = protocolIDs[1:]

	if len(payload) < 2 {
		return nil, nil, nil, errors.Wrapf(ErrInvalidChannels, "not enough signature push ops: %d",
			len(payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload[0])
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, nil, nil, errors.Wrap(ErrUnsupportedSignedMessagesVersion,
			fmt.Sprintf("%d", version))
	}
	payload = payload[1:]

	signature := &Signature{}
	payload, err = bsor.Unmarshal(payload, signature)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "unmarshal")
	}

	hasher := sha256.New()
	for _, protocolID := range protocolIDs {
		hasher.Write(protocolID)
	}
	if err := payload.Write(hasher); err != nil {
		return nil, nil, nil, errors.Wrap(err, "hash")
	}

	hash, err := bitcoin.NewHash32(hasher.Sum(nil))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "new hash")
	}
	signature.hash = hash

	return signature, protocolIDs, payload, nil
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
