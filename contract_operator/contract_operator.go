package contract_operator

import (
	"bytes"
	"fmt"

	"github.com/tokenized/channels"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/expanded_tx"
	"github.com/tokenized/pkg/peer_channels"

	"github.com/pkg/errors"
)

var (
	ProtocolID = envelope.ProtocolID("TKN.OPERATOR") // Protocol ID for contract operator messages
	Version    = uint8(0)

	MessageTypeInvalid     = MessageType(0)
	MessageTypeCreateAgent = MessageType(1)
	MessageTypeAgent       = MessageType(2)
	MessageTypeSignTx      = MessageType(3)
	MessageTypeSignedTx    = MessageType(4)

	// ResponseCodeMissingAdminInput means the tx to sign does not yet have an administrator input.
	ResponseCodeMissingAdminInput = uint32(1)

	// ResponseCodeMissingContractOutput means the tx to sign does not yet have a contract output.
	ResponseCodeMissingContractOutput = uint32(2)

	// ResponseCodeContractActionInvalid means the contract action in the tx to sign is invalid.
	ResponseCodeContractActionInvalid = uint32(3)

	ErrUnsupportedVersion                 = errors.New("Unsupported Operator Version")
	ErrUnsupportedContractOperatorMessage = errors.New("Unsupported Operator Message")
)

type MessageType uint8

type Protocol struct{}

func NewProtocol() *Protocol {
	return &Protocol{}
}

func (*Protocol) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (*Protocol) Parse(payload envelope.Data) (channels.Message, envelope.Data, error) {
	return Parse(payload)
}

func (*Protocol) ResponseCodeToString(code uint32) string {
	return ResponseCodeToString(code)
}

func ResponseCodeToString(code uint32) string {
	switch code {
	case ResponseCodeMissingAdminInput:
		return "missing_admin_input"
	case ResponseCodeMissingContractOutput:
		return "missing_contract_output"
	case ResponseCodeContractActionInvalid:
		return "contract_action_invalid"
	default:
		return "parse_error"
	}
}

type CreateAgent struct {
	// AdminLockingScript is the locking script that will be given administrative control of the
	// created smart contract agent. The SignTx request and any other admin smart contract agent
	// requests must use this locking script.
	AdminLockingScript bitcoin.Script `bsor:"1" json:"admin_locking_script"`

	// MasterLockingScript is the locking script used to change the contract locking script in the
	// worst case scenario that the contract locking script is compromised or the contract operator
	// refuses to provide services. When left empty it will be provided by the contract operator.
	MasterLockingScript bitcoin.Script `bsor:"2" json:"master_locking_script"`

	// MinimumContractFee is the minimum fee, in satoshis, that is charged by the smart contract
	// agent for every action response. The smart contract agent will reject any contract offers or
	// contract amendments that try to set the value lower.
	MinimumContractFee uint64 `bsor:"3" json:"contract_fee"`

	// FeeLockingScript is the locking script to pay contract fees to. This should normally be
	// controlled by and paid to the contract operator, but special deals can be made to pay to
	// other parties. When left empty it will be provided by the contract operator.
	FeeLockingScript bitcoin.Script `bsor:"4" json:"fee_locking_script"`
}

func (*CreateAgent) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *CreateAgent) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeCreateAgent)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

type Agent struct {
	LockingScript       bitcoin.Script         `bsor:"1" json:"locking_script"`
	MinimumContractFee  uint64                 `bsor:"2" json:"minimum_contract_fee"`
	MasterLockingScript bitcoin.Script         `bsor:"3" json:"master_locking_script"`
	PeerChannel         *peer_channels.Channel `bsor:"4" json:"peer_channel"`
}

func (*Agent) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *Agent) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeAgent)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

type SignTx struct {
	Tx *expanded_tx.ExpandedTx `bsor:"1" json:"locking_script"`
}

func (*SignTx) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *SignTx) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeSignTx)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

type SignedTx struct {
	Tx *expanded_tx.ExpandedTx `bsor:"1" json:"locking_script"`
}

func (*SignedTx) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *SignedTx) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeSignedTx)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

func Parse(payload envelope.Data) (channels.Message, envelope.Data, error) {
	if len(payload.ProtocolIDs) == 0 {
		return nil, payload, nil
	}

	if !bytes.Equal(payload.ProtocolIDs[0], ProtocolID) {
		return nil, payload, nil
	}

	if len(payload.ProtocolIDs) != 1 {
		return nil, payload, errors.Wrapf(channels.ErrInvalidMessage,
			"contract operator can't wrap")
	}
	payload.ProtocolIDs = payload.ProtocolIDs[1:]

	if len(payload.Payload) == 0 {
		return nil, payload, errors.Wrapf(channels.ErrInvalidMessage, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, payload, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, payload, errors.Wrap(ErrUnsupportedVersion, fmt.Sprintf("%d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload.Payload[1])
	if err != nil {
		return nil, payload, errors.Wrap(err, "message type")
	}

	result := MessageForType(MessageType(messageType))
	if result == nil {
		return nil, payload, errors.Wrap(ErrUnsupportedContractOperatorMessage,
			fmt.Sprintf("%d", MessageType(messageType)))
	}

	payloads, err := bsor.Unmarshal(payload.Payload[2:], result)
	if err != nil {
		return nil, payload, errors.Wrap(err, "unmarshal")
	}
	payload.Payload = payloads

	return result, payload, nil
}

func MessageForType(messageType MessageType) channels.Message {
	switch messageType {
	case MessageTypeCreateAgent:
		return &CreateAgent{}
	case MessageTypeAgent:
		return &Agent{}
	case MessageTypeSignTx:
		return &SignTx{}
	case MessageTypeSignedTx:
		return &SignedTx{}
	case MessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func MessageTypeFor(message channels.Message) MessageType {
	switch message.(type) {
	case *CreateAgent:
		return MessageTypeCreateAgent
	case *Agent:
		return MessageTypeAgent
	case *SignTx:
		return MessageTypeSignTx
	case *SignedTx:
		return MessageTypeSignedTx
	default:
		return MessageTypeInvalid
	}
}

func (v *MessageType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for MessageType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v MessageType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v MessageType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown MessageType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *MessageType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *MessageType) SetString(s string) error {
	switch s {
	case "create_agent":
		*v = MessageTypeCreateAgent
	case "agent":
		*v = MessageTypeAgent
	case "sign_tx":
		*v = MessageTypeSignTx
	case "signed_tx":
		*v = MessageTypeSignedTx
	default:
		*v = MessageTypeInvalid
		return fmt.Errorf("Unknown MessageType value \"%s\"", s)
	}

	return nil
}

func (v MessageType) String() string {
	switch v {
	case MessageTypeCreateAgent:
		return "create_agent"
	case MessageTypeAgent:
		return "agent"
	case MessageTypeSignTx:
		return "sign_tx"
	case MessageTypeSignedTx:
		return "signed_tx"
	default:
		return ""
	}
}
