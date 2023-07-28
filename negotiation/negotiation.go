package negotiation

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/tokenized/channels"
	channelsExpandedTx "github.com/tokenized/channels/expanded_tx"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsvalias"
	"github.com/tokenized/pkg/expanded_tx"
	"github.com/tokenized/pkg/fees"

	"github.com/pkg/errors"
)

const (
	StatusComplete             = Status(0x00)
	StatusNeedsSigned          = Status(0x01)
	StatusNeedsOutputs         = Status(0x02)
	StatusNeedsInputs          = Status(0x04)
	StatusNeedsReceivers       = Status(0x08)
	StatusNeedsSenders         = Status(0x10)
	StatusNeedsSignedAndInputs = StatusNeedsSigned | StatusNeedsInputs
)

type Transaction struct {
	// ThreadID is a unique "conversation" ID for the negotiation. Responses should include the same
	// ID. UUIDs are recommended.
	ThreadID *string `json:"thread_id,omitempty"`

	// Fees specifies any requirements for fees when modifying the transaction.
	Fees fees.FeeRequirements `json:"fees,omitempty"`

	// ReplyTo is information on how to respond to the message.
	ReplyTo *channels.ReplyTo `json:"reply_to,omitempty"`

	// Note is optional text that is displayed to the user.
	Note *string `json:"note,omitempty"`

	// Expiry is the nanoseconds since the unix epoch until this transaction expires.
	Expiry *channels.Time `json:"expiry,omitempty"`

	// Timestamp is the nanoseconds since the unix epoch until when this transaction was created.
	Timestamp *channels.Time `json:"timestamp,omitempty"`

	Response *channels.Response `json:"response,omitempty"`

	// Tx is the current state of the negotiation. It will start as a partial transaction, likely
	// missing inputs and/or outputs.
	Tx *expanded_tx.ExpandedTx `json:"expanded_tx,omitempty"`
}

type Status uint8

func ConvertFromBSVAlias(bntx *bsvalias.NegotiationTransaction) *Transaction {
	result := &Transaction{
		ThreadID: bntx.ThreadID,
		Fees:     bntx.Fees,
		Note:     bntx.Note,
		Tx:       bntx.Tx,
	}

	if bntx.Timestamp != nil {
		ts := channels.Time(*bntx.Timestamp)
		result.Timestamp = &ts
	}

	if bntx.Expiry != nil {
		e := channels.Time(*bntx.Expiry)
		result.Expiry = &e
	}

	if bntx.ReplyTo != nil {
		result.ReplyTo = &channels.ReplyTo{
			Handle:      bntx.ReplyTo.Handle,
			PeerChannel: bntx.ReplyTo.PeerChannel,
		}
	}

	if bntx.Response != nil {
		result.Response = &channels.Response{
			Status:         channels.Status(bntx.Response.Status),
			CodeProtocolID: envelope.ProtocolID(bntx.Response.CodeProtocolID),
			Code:           bntx.Response.Code,
			Note:           bntx.Response.Note,
		}
	}

	return result
}

func (tx *Transaction) ConvertToBSVAlias() *bsvalias.NegotiationTransaction {
	result := &bsvalias.NegotiationTransaction{
		ThreadID: tx.ThreadID,
		Fees:     tx.Fees,
		Note:     tx.Note,
		Tx:       tx.Tx,
	}

	if tx.Timestamp != nil {
		ts := uint64(*tx.Timestamp)
		result.Timestamp = &ts
	}

	if tx.Expiry != nil {
		e := uint64(*tx.Expiry)
		result.Expiry = &e
	}

	if tx.ReplyTo != nil {
		result.ReplyTo = &bsvalias.ReplyTo{
			Handle:      tx.ReplyTo.Handle,
			PeerChannel: tx.ReplyTo.PeerChannel,
		}
	}

	if tx.Response != nil {
		result.Response = &bsvalias.Response{
			Status:         bsvalias.Status(tx.Response.Status),
			CodeProtocolID: bitcoin.Hex(tx.Response.CodeProtocolID),
			Code:           tx.Response.Code,
			Note:           tx.Response.Note,
		}
	}

	return result
}

// CompileNegotiationTransaction extracts relevant values from a parsed script into the negotiation
// transaction. It returns any unused wrappers.
func CompileTransaction(message channels.Message,
	wrappers []channels.Wrapper) (*Transaction, []channels.Wrapper, error) {

	channelEtx, ok := message.(*channelsExpandedTx.ExpandedTxMessage)
	if !ok {
		return nil, nil, errors.New("Not Expanded Tx")
	}

	result := &Transaction{
		Tx: channelEtx.GetTx(),
	}

	if result.Tx == nil {
		return nil, nil, errors.New("Missing Tx")
	}

	var unusedWrappers []channels.Wrapper
	for _, wrapper := range wrappers {
		switch m := wrapper.(type) {
		case *channels.StringID:
			result.ThreadID = &m.StringID
		case *channels.FeeRequirementsMessage:
			result.Fees = m.GetFeeRequirements()
		case *channels.ReplyTo:
			result.ReplyTo = m
		case *channels.Response:
			result.Response = m
		case *channels.Note:
			result.Note = &m.Note
		case *channels.TimeMessage:
			t := m.GetTime()
			result.Timestamp = &t
		case *channels.ExpiryMessage:
			t := m.GetExpiry()
			result.Expiry = &t
		default:
			unusedWrappers = append(unusedWrappers, wrapper)
		}
	}

	return result, unusedWrappers, nil
}

func (tx Transaction) Copy() Transaction {
	result := Transaction{
		Fees: tx.Fees.Copy(),
	}

	if tx.ThreadID != nil {
		c := CopyString(*tx.ThreadID)
		result.ThreadID = &c
	}

	if tx.ReplyTo != nil {
		c := tx.ReplyTo.Copy()
		result.ReplyTo = &c
	}

	if tx.Note != nil {
		c := CopyString(*tx.Note)
		result.Note = &c
	}

	if tx.Expiry != nil {
		c := tx.Expiry.Copy()
		result.Expiry = &c
	}

	if tx.Timestamp != nil {
		c := tx.Timestamp.Copy()
		result.Timestamp = &c
	}

	if tx.Tx != nil {
		c := tx.Tx.Copy()
		result.Tx = &c
	}

	if tx.Response != nil {
		c := tx.Response.Copy()
		result.Response = &c
	}

	return result
}

func CopyString(s string) string {
	result := make([]byte, len(s))
	copy(result, s)
	return string(result)
}

func (m *Transaction) Write() (envelope.Data, error) {
	var wrappers []channels.Wrapper

	if m.ThreadID != nil {
		wrappers = append(wrappers, channels.NewStringID(*m.ThreadID))
	}

	if m.Fees != nil {
		wrappers = append(wrappers, channels.NewFeeRequirementsMessage(m.Fees))
	}

	if m.ReplyTo != nil {
		wrappers = append(wrappers, m.ReplyTo)
	}

	if m.Note != nil {
		wrappers = append(wrappers, channels.NewNote(*m.Note))
	}

	if m.Expiry != nil {
		wrappers = append(wrappers, channels.NewExpiryMessage(*m.Expiry))
	}

	if m.Timestamp != nil {
		wrappers = append(wrappers, channels.NewTimeMessage(*m.Timestamp))
	}

	if m.Tx != nil {
		if m.Response != nil {
			wrappers = append(wrappers, m.Response)
		}

		cetx := channelsExpandedTx.NewExpandedTxMessage(m.Tx)
		return channels.WrapEnvelope(cetx, wrappers...)
	}

	if m.Response != nil {
		return channels.WrapEnvelope(m.Response, wrappers...)
	}

	return envelope.Data{}, errors.New("Missing tx and response")
}

func (m *Transaction) Wrap() (bitcoin.Script, error) {
	data, err := m.Write()
	if err != nil {
		return nil, errors.Wrap(err, "envelope")
	}

	return envelopeV1.Wrap(data).Script()
}

func TxIsSigned(tx expanded_tx.TransactionWithOutputs) bool {
	inputCount := tx.InputCount()
	if inputCount == 0 {
		return false
	}

	for i := 0; i < inputCount; i++ {
		input := tx.Input(i)
		if len(input.UnlockingScript) == 0 {
			return false
		}
	}

	return true
}

// // TxAction will return the action of the tx. The action being anything other than a message where
// // it is only valid to have one per tx.
// func TxAction(tx expanded_tx.Transaction, isTest bool) actions.Action {
// 	outputCount := tx.OutputCount()
// 	for index := 0; index < outputCount; index++ {
// 		output := tx.Output(index)

// 		action, err := protocol.Deserialize(output.LockingScript, isTest)
// 		if err != nil {
// 			continue
// 		}

// 		if action.Code() != actions.CodeMessage {
// 			return action
// 		}
// 	}

// 	return nil
// }

// TxIsComplete returns true if sent quantities approximately match receive quantities. If there is
// a Tokenized transfer then sender quantities must match receiver quantities.
// maxFeeRate specifies the maximum fee rate that will be considered complete. A fee rate over max
// means that the tx likely needs more bitcoin receivers.
// isTest specifies which type of Tokenized actions to look for.
func TxStatus(tx expanded_tx.TransactionWithOutputs, maxFeeRate float64,
	isTest bool) (Status, error) {

	var status Status

	inputCount := tx.InputCount()
	inputValue := uint64(0)
	if inputCount == 0 {
		status |= StatusNeedsInputs
	} else {
		for index := 0; index < inputCount; index++ {
			output, err := tx.InputOutput(index)
			if err != nil {
				return status, errors.Wrapf(err, "input %d", index)
			}

			inputValue += output.Value
		}
	}

	outputValue := uint64(0)
	outputCount := tx.OutputCount()
	// var transfer *actions.Transfer
	if outputCount == 0 {
		status |= StatusNeedsOutputs
	} else {
		for index := 0; index < outputCount; index++ {
			output := tx.Output(index)
			outputValue += output.Value

			// action, err := protocol.Deserialize(output.LockingScript, isTest)
			// if err != nil {
			// 	continue
			// }

			// if tfr, ok := action.(*actions.Transfer); ok {
			// 	if transfer != nil {
			// 		return status, errors.New("More than one transfer")
			// 	}
			// 	transfer = tfr
			// }
		}
	}

	if outputValue > inputValue {
		status |= StatusNeedsInputs
	} else {
		txSize := tx.GetMsgTx().SerializeSize()
		fee := inputValue - outputValue
		feeRate := float64(fee) / float64(txSize)
		if feeRate > maxFeeRate {
			status |= StatusNeedsOutputs
		}
	}

	// for _, instrumentTransfer := range transfer.Instruments {
	// 	senderQuantity := uint64(0)
	// 	for _, sender := range instrumentTransfer.InstrumentSenders {
	// 		senderQuantity += sender.Quantity
	// 	}

	// 	receiverQuantity := uint64(0)
	// 	for _, receiver := range instrumentTransfer.InstrumentReceivers {
	// 		receiverQuantity += receiver.Quantity
	// 	}

	// 	if senderQuantity > receiverQuantity {
	// 		status |= StatusNeedsReceivers
	// 	} else if receiverQuantity > senderQuantity {
	// 		status |= StatusNeedsSenders
	// 	}
	// }

	return status, nil
}

func (v Status) IsExchangeRequest() bool {
	if v&StatusNeedsInputs != 0 &&
		v&StatusNeedsReceivers != 0 {
		// Exchange of bitcoin for tokens
		return true
	}

	if v&StatusNeedsOutputs != 0 &&
		v&StatusNeedsSenders != 0 {
		// Exchange of tokens for bitcoin
		return true
	}

	if v&StatusNeedsSenders != 0 &&
		v&StatusNeedsReceivers != 0 {
		// Exchange of tokens for tokens
		return true
	}

	return false
}

func (v Status) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown Status value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *Status) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *Status) SetString(s string) error {
	parts := strings.Split(s, "|")
	value := Status(0)
	for _, part := range parts {
		switch s {
		case "complete":
			*v = StatusComplete
			return nil
		case "needs_signed":
			value |= StatusNeedsSigned
		case "needs_outputs":
			value |= StatusNeedsOutputs
		case "needs_inputs":
			value |= StatusNeedsInputs
		case "needs_receivers":
			value |= StatusNeedsReceivers
		case "needs_senders":
			value |= StatusNeedsSenders
		default:
			*v = 0
			return fmt.Errorf("Unknown Status value \"%s\"", part)
		}
	}

	*v = value
	return nil
}

func (v Status) String() string {
	if v == StatusComplete {
		return "complete"
	}

	var values []string
	if v&StatusNeedsSigned != 0 {
		values = append(values, "needs_signed")
	}
	if v&StatusNeedsOutputs != 0 {
		values = append(values, "needs_outputs")
	}
	if v&StatusNeedsInputs != 0 {
		values = append(values, "needs_inputs")
	}
	if v&StatusNeedsReceivers != 0 {
		values = append(values, "needs_receivers")
	}
	if v&StatusNeedsSenders != 0 {
		values = append(values, "needs_senders")
	}

	return strings.Join(values, "|")
}

// Scan converts from a database column.
func (v *Status) Scan(data interface{}) error {
	value := reflect.ValueOf(data)
	switch value.Type().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		*v = Status(value.Int())
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		*v = Status(value.Uint())
		return nil
	default:
		return errors.New("Status db column not an integer")
	}
}
