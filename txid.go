package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"

	"github.com/pkg/errors"
)

const (
	TxIDVersion = uint8(0)
)

var (
	ProtocolIDTxID = envelope.ProtocolID("TxID") // Protocol ID for txid

	ErrInvalidTxID = errors.New("Invalid TxID")
)

type TxIDProtocol struct{}

func NewTxIDProtocol() *TxIDProtocol {
	return &TxIDProtocol{}
}

func (*TxIDProtocol) ProtocolID() envelope.ProtocolID {
	return ProtocolIDTxID
}

func (*TxIDProtocol) Parse(payload envelope.Data) (Message, envelope.Data, error) {
	return ParseTxID(payload)
}

func (*TxIDProtocol) ResponseCodeToString(code uint32) string {
	return "parse"
}

type TxID bitcoin.Hash32

func (*TxID) IsWrapperType() {}

func (*TxID) ProtocolID() envelope.ProtocolID {
	return ProtocolIDTxID
}

func (m *TxID) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(TxIDVersion))}

	// Message
	payload = append(payload, bitcoin.NewPushDataScriptItem(m[:]))

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDTxID}, payload}, nil
}

// WrapTxID wraps the payload with the txid and returns the new payload containing the txid.
func WrapTxID(payload envelope.Data, id bitcoin.Hash32) (envelope.Data, error) {
	txid := TxID(id)
	return txid.Wrap(payload)
}

func (m *TxID) Wrap(payload envelope.Data) (envelope.Data, error) {
	// Version
	scriptItems := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(TxIDVersion))}

	// Message
	scriptItems = append(scriptItems, bitcoin.NewPushDataScriptItem(m[:]))

	payload.ProtocolIDs = append(envelope.ProtocolIDs{ProtocolIDTxID}, payload.ProtocolIDs...)
	payload.Payload = append(scriptItems, payload.Payload...)

	return payload, nil
}

func ParseTxID(payload envelope.Data) (*TxID, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 || !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDTxID) {
		return nil, payload, nil
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) < 2 {
		return nil, payload, errors.Wrapf(ErrInvalidMessage, "not enough txid push ops: %d",
			len(payload.Payload))
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion, fmt.Sprintf("txid: %d", version))
	}

	if payload.Payload[1].Type != bitcoin.ScriptItemTypePushData {
		return nil, payload, errors.Wrap(ErrInvalidTxID, "not push data")
	}

	var result TxID
	if len(payload.Payload[1].Data) != len(result[:]) {
		return nil, payload, errors.Wrapf(ErrInvalidTxID, "wrong size: %d",
			len(payload.Payload[1].Data))
	}
	copy(result[:], payload.Payload[1].Data)

	payload.Payload = payload.Payload[2:]
	return &result, payload, nil
}
