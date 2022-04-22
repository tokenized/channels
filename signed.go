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
	Signature bitcoin.Signature  `bsor:"1" json:"signature"`
	PublicKey *bitcoin.PublicKey `bsor:"2" json:"public_key"`
	hash      *bitcoin.Hash32
}

// Sign adds the SignedMessage protocol to the provided bitcoin script with the signature of the
// script and the key is specified.
func Sign(protocolIDs envelope.ProtocolIDs, payload bitcoin.ScriptItems, key bitcoin.Key,
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

	signature, err := key.Sign(*hash)
	if err != nil {
		return nil, nil, errors.Wrap(err, "sign")
	}

	// Version
	result := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(SignedMessagesVersion))}

	message := &Signature{
		Signature: signature,
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

	if len(protocolIDs) < 2 {
		return nil, nil, nil, errors.Wrapf(ErrNotSignedMessage, "minimum 2 protocols: %d",
			len(protocolIDs))
	}

	if !bytes.Equal(protocolIDs[0], ProtocolIDSignedMessages) {
		return nil, nil, nil, errors.Wrapf(ErrNotSignedMessage, "wrong protocol id: 0x%x",
			protocolIDs[0])
	}
	protocolIDs = protocolIDs[1:]

	if len(payload) < 1 {
		return nil, nil, nil, errors.Wrapf(ErrNotSignedMessage, "minimum 4 push ops: %d",
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

// SetPublicKey sets the public key on the message. To be used when the public key isn't provided in
// the message.
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

	if !m.Signature.Verify(*m.hash, *m.PublicKey) {
		return ErrInvalidSignature
	}

	return nil
}
