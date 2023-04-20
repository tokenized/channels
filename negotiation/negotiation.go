package negotiation

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/tokenized/pkg/expanded_tx"

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

type Status uint8

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
