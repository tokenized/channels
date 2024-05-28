package authorize_script

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tokenized/bitcoin_interpreter"
	"github.com/tokenized/bitcoin_interpreter/p2pk"
	"github.com/tokenized/channels"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"

	"github.com/pkg/errors"
)

const (
	AuthorizeScriptVersion = uint8(0)

	// AuthorizeStatusAuthorizeRequired is a code specific to the signature protocol that is placed
	// in a Reject message to signify that a message is considered invalid if it is not signed.
	AuthorizeStatusAuthorizeRequired = uint32(1)

	// AuthorizeStatusInvalidAuthorize is a code specific to the signature protocol that is placed
	// in a Reject message to signify that a message has an invalid signature. Either the wrong
	// key was used or the signature is just invalid.
	AuthorizeStatusNotUnlocked = uint32(2)
)

var (
	ProtocolID = envelope.ProtocolID("A") // Protocol ID for signed messages

	ErrPayloadMissing = errors.New("Payload Missing")
)

// AuthorizeProtocol is a protocol for authorizing data sets with Bitcoin locking and unlocking
// scripts. Authorize hashes are calculated on the data set so that the "unlocking" is specific to
// the data set contained. A standard locking script is just a public key followed by OP_CHECKSIG
// and a standard unlocking script is just the corresponding signature, but since it is a script it
// can easily support multi-sig and more complex authorization schemes.
type AuthorizeProtocol struct{}

func NewProtocol() *AuthorizeProtocol {
	return &AuthorizeProtocol{}
}

func (*AuthorizeProtocol) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (*AuthorizeProtocol) Parse(payload envelope.Data) (channels.Message, envelope.Data, error) {
	return ParseAuthorize(payload)
}

func (*AuthorizeProtocol) ResponseCodeToString(code uint32) string {
	return AuthorizeResponseCodeToString(code)
}

// Authorize is the payload wrapped around a data set to show the authorization for the data set.
type Authorize struct {
	LockingScript   bitcoin.Script `bsor:"1" json:"locking_script"`
	UnlockingScript bitcoin.Script `bsor:"2" json:"unlocking_script"`

	preimage []byte
	unlocker bitcoin_interpreter.Unlocker
	sigHash  *bitcoin.Hash32
}

func NewAuthorize(lockingScript bitcoin.Script,
	unlocker bitcoin_interpreter.Unlocker) (*Authorize, error) {

	return &Authorize{
		LockingScript: lockingScript,
		unlocker:      unlocker,
	}, nil
}

// NewAuthorizeP2PK creates an authorize script using the standard OP_CHECKSIG script for a single
// key.
func NewAuthorizeP2PK(key bitcoin.Key) (*Authorize, error) {
	unlocker := p2pk.NewUnlocker(key)
	lockingScript := p2pk.CreateScript(key.PublicKey(), false)

	return &Authorize{
		LockingScript: lockingScript,
		unlocker:      unlocker,
	}, nil
}

func (*Authorize) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *Authorize) Wrap(payload envelope.Data) (envelope.Data, error) {
	if m.unlocker != nil {
		if err := m.authorize(payload); err != nil {
			return payload, errors.Wrap(err, "authorize")
		}
	}

	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(AuthorizeScriptVersion))}

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return payload, errors.Wrap(err, "marshal")
	}
	scriptItems = append(scriptItems, msgScriptItems...)

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolID}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func (m *Authorize) authorize(payload envelope.Data) error {
	if err := m.SetPayload(payload); err != nil {
		return errors.Wrap(err, "set payload")
	}

	unlockingScript, err := m.unlocker.Unlock(m.WriteSignaturePreimage, m.LockingScript)
	if err != nil {
		return errors.Wrap(err, "unlock")
	}
	m.UnlockingScript = unlockingScript

	return nil
}

func AuthorizePayload(payload envelope.Data, lockingScript bitcoin.Script,
	unlocker bitcoin_interpreter.Unlocker) (*Authorize, error) {

	result := &Authorize{
		LockingScript: lockingScript,
	}

	if err := result.SetPayload(payload); err != nil {
		return nil, errors.Wrap(err, "set payload")
	}

	unlockingScript, err := unlocker.Unlock(result.WriteSignaturePreimage, lockingScript)
	if err != nil {
		return nil, errors.Wrap(err, "unlock")
	}
	result.UnlockingScript = unlockingScript

	return result, nil
}

// AuthorizeP2PK adds the AuthorizeMessage protocol to the provided bitcoin script with the
// standard OP_CHECKSIG script for a single key.
func AuthorizeP2PK(payload envelope.Data, key bitcoin.Key) (*Authorize, error) {
	unlocker := p2pk.NewUnlocker(key)
	lockingScript := p2pk.CreateScript(key.PublicKey(), false)

	result := &Authorize{
		LockingScript: lockingScript,
	}

	if err := result.SetPayload(payload); err != nil {
		return nil, errors.Wrap(err, "set payload")
	}

	unlockingScript, err := unlocker.Unlock(result.WriteSignaturePreimage, lockingScript)
	if err != nil {
		return nil, errors.Wrap(err, "unlock")
	}
	result.UnlockingScript = unlockingScript

	return result, nil
}

func WrapAuthorize(payload envelope.Data, lockingScript bitcoin.Script,
	unlocker bitcoin_interpreter.Unlocker) (envelope.Data, error) {

	wrapper, err := AuthorizePayload(payload, lockingScript, unlocker)
	if err != nil {
		return payload, errors.Wrap(err, "authorize")
	}

	return wrapper.Wrap(payload)
}

// WrapAuthorize signs the payload and wraps the payload with the signature and returns the new
// payload containing the signature.
func WrapAuthorizeP2PK(payload envelope.Data, key bitcoin.Key) (envelope.Data, error) {
	wrapper, err := AuthorizeP2PK(payload, key)
	if err != nil {
		return payload, errors.Wrap(err, "authorize")
	}

	return wrapper.Wrap(payload)
}

// ParseAuthorize parses the signature and public key (if provided).
func ParseAuthorize(payload envelope.Data) (*Authorize, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 || !bytes.Equal(payload.ProtocolIDs[0], ProtocolID) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 2 {
		return nil, payload, errors.Wrapf(channels.ErrInvalidMessage,
			"not enough signature push ops: %d", len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(channels.ErrUnsupportedVersion,
			fmt.Sprintf("signed: %d", version))
	}
	payload.Payload = payload.Payload[1:]

	result := &Authorize{}
	payload.Payload, err = bsor.Unmarshal(payload.Payload, result)
	if err != nil {
		return nil, payload, errors.Wrap(err, "unmarshal")
	}

	if err := result.SetPayload(payload); err != nil {
		return nil, payload, errors.Wrap(err, "set payload")
	}

	return result, payload, nil
}

func (m *Authorize) SetPayload(payload envelope.Data) error {
	buf := &bytes.Buffer{}
	for _, protocolID := range payload.ProtocolIDs {
		if _, err := buf.Write(protocolID); err != nil {
			return errors.Wrap(err, "protocol id")
		}
	}

	if err := payload.Payload.Write(buf); err != nil {
		return errors.Wrap(err, "payload")
	}

	m.preimage = buf.Bytes()
	return nil
}

func (m Authorize) WriteSignaturePreimage(w io.Writer, hashType bitcoin_interpreter.SigHashType,
	codeScript bitcoin.Script, opCodeSeparatorIndex int) error {

	if _, err := w.Write(m.preimage); err != nil {
		return errors.Wrap(err, "write")
	}

	return nil
}

func (m Authorize) Verify() error {
	if len(m.preimage) == 0 {
		return ErrPayloadMissing
	}

	if err := bitcoin_interpreter.Verify(nil, m.WriteSignaturePreimage, m.LockingScript,
		m.UnlockingScript); err != nil {
		return errors.Wrap(err, "verify")
	}

	return nil
}

func AuthorizeResponseCodeToString(code uint32) string {
	switch code {
	case AuthorizeStatusAuthorizeRequired:
		return "authorization_required"
	case AuthorizeStatusNotUnlocked:
		return "not_unlocked"
	default:
		return "parse_error"
	}
}
