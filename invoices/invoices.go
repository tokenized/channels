package invoices

import (
	"bytes"
	"fmt"

	"github.com/tokenized/channels"
	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/expanded_tx"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

const (
	Version = uint8(0)

	MessageTypeInvalid         = MessageType(0)
	MessageTypeRequestMenu     = MessageType(1)
	MessageTypeMenu            = MessageType(2)
	MessageTypePurchaseOrder   = MessageType(3)
	MessageTypeInvoice         = MessageType(4)
	MessageTypeTransferRequest = MessageType(5)
	MessageTypeTransfer        = MessageType(6)
	MessageTypeTransferAccept  = MessageType(7)

	// StatusTxNotAccepted is a code specific to the invoices protocol that is placed
	// in a Reject message to signify that a Bitcoin transaction was not accepted by the network.
	StatusTxNotAccepted = uint32(1)
	StatusInvalidOrder  = uint32(2)
	StatusUnknownItem   = uint32(3)
	StatusWrongPrice    = uint32(4)

	// StatusTxNotValid means the transaction or its ancestors did not validate. There
	// could be an invalid signature or parse error in the tx, or one of its ancestors.
	StatusTxNotValid = uint32(5)

	// StatusTxFeeTooLow means the tx does not pay the fee requirement defined in the
	// previously provided fee quote.
	StatusTxFeeTooLow = uint32(6)

	// StatusTxMissingAncestor means the `InvoicesRequirementAncestorsToMerkleProofs`
	// requirement was not met. Ancestors were not provided all the way to merkle proofs.
	StatusTxMissingAncestor = uint32(7)

	// StatusTxMissingInput means the ancestors do not meet the
	// InvoicesRequirementInputs requirement. Not all outputs being spent by the tx are included in
	// the ancestors.
	StatusTxMissingInput = uint32(8)

	// StatusTransferUnknown means that a received Transfer does not match any
	// previously sent TransferRequest.
	StatusTransferUnknown = uint32(9)

	// StatusMissingResponseID means that a message required a response id to be valid.
	StatusMissingResponseID = uint32(10)
)

var (
	// OptionRequireInputs specifies that all expanded txs must contain all parents. Meaning
	// the transactions containing the outputs being spent by the inputs of the expanded tx. They do
	// not require merkle proofs for them, but the tx must be included.
	OptionRequireInputs = bitcoin.Hex{0x01}

	// OptionRequireAncestorsToMerkleProofs specifies that all expanded txs must contain all
	// ancestors in the ancestry tree up to the nearest merkle proof. Meaning all leaves of the
	// ancestry tree must have merkle proofs.
	OptionRequireAncestorsToMerkleProofs = bitcoin.Hex{0x02}
)

var (
	ProtocolID = envelope.ProtocolID("I") // Protocol ID for invoice negotiation

	TokenProtocolBitcoin = []byte("Bitcoin")

	ErrUnsupportedInvoicesMessage = errors.New("Unsupported Invoices Message")
	ErrInvoiceMissing             = errors.New("Invoice Missing")
)

type MessageType uint8

type Protocol struct{}

func NewProtocol() *Protocol {
	return &Protocol{}
}

func (*Protocol) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (*Protocol) Parse(payload envelope.Data) (channels.Message, error) {
	return Parse(payload)
}

func (*Protocol) ResponseCodeToString(code uint32) string {
	return ResponseCodeToString(code)
}

// Invoices provides a method for negotiating payments for products or services.
// Vendor Workflow:
//   1. Vendor sends Menu containing available Services/Products.
//   2. Buyer sends PurchaseOrder with requested Services/Products from the menu.
//   3. A. Vendor approves and sends the buyer an TransferRequest containing an Invoice corresponding to
//      the PurchaseOrder and
//      B. Vendor rejects and sends a modified PurchaseOrder for buyer approval. This negotiation
//      can continue indefinitely.
//   4. If the vendor approved then the buyer sends a Transfer that embeds the invoice
//     otherwise the buyer either quits, modifies the invoice, or keeps the same invoice and sends
//     it back to the vendor.
//
// User to User Workflow (Request To Send Payment):
//   1. User A sends either a PurchaseOrder to request to pay User B. The purchase order describes
//   the purpose of the payment.
//   2. User B responds with an TransferRequest to specify how User A should make the payment. The
//   TransferRequest is an incomplete tx paying User B and optionally an Invoice output describing the
//   purpose of the payment.
//   3. User A completes the transaction by adding inputs and other payment information required,
//   signs it, and responds with an Transfer message.
//
// User to User Workflow (Request To Receive Payment):
//   1. User A sends an TransferRequest to request payment from User B. The TransferRequest contains an
//   incomplete tx paying User A and optionally an Invoice output describing the purpose of the
//   payment.
//   2. User B completes the transaction by adding inputs and other payment information required,
//   signs it, and responds with an Transfer message.
//   3. User A signs any inputs they might have on the transaction and broadcasts it.

// RequestMenu is a request to receive the current menu.
type RequestMenu struct {
}

func (*RequestMenu) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *RequestMenu) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeRequestMenu)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

// Menu represents a set of items available to include in an invoice.
type Menu struct {
	Items Items `bsor:"1" json:"items"`
}

func (*Menu) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *Menu) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeMenu)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

// PurchaseOrder contains items the buyer wishes to purchase.
// Identity is implicit based on the relationship and the key that signed the message.
type PurchaseOrder struct {
	Items InvoiceItems `bsor:"1" json:"items"`
	Notes *string      `bsor:"2" json:"notes,omitempty"`
}

func (*PurchaseOrder) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *PurchaseOrder) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypePurchaseOrder)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

// Invoice is a message created by the vendor representing an approved set of items to buy. This is
// meant to be embedded in the payment tx so what is being paid for is recorded with the payment. It
// can be encrypted for privacy.
// Identity is implicit based on the peer channel relationship and the key that signed the message.
// The vendor can either add an input to the payment tx to sign it directly, or the buyer can retain
// signatures from the off chain communication to prove the vendor approved the payment. The off
// chain communication should include a signed TransferRequest message that contains the payment tx
// which contains the Invoice.
type Invoice struct {
	Items      InvoiceItems       `bsor:"1" json:"items"`
	Notes      *string            `bsor:"2" json:"notes,omitempty"`
	Timestamp  channels.Timestamp `bsor:"3" json:"timestamp"`
	Expiration channels.Timestamp `bsor:"4" json:"expiration"`
}

func (*Invoice) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *Invoice) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeInvoice)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

// TransferRequest is an incomplete tx that includes an output containing the Invoice message and
// transfers of requested tokens/bitcoin for the items contained in the invoice.
type TransferRequest struct {
	Tx   *expanded_tx.ExpandedTx  `bsor:"1" json:"tx"`
	Fees channels.FeeRequirements `bsor:"2" json:"fees"` // tx fee requirements
}

func (*TransferRequest) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *TransferRequest) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(MessageTypeTransferRequest)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

// Transfer is a payment transaction that embeds the approved invoice.
type Transfer struct {
	Tx *expanded_tx.ExpandedTx `bsor:"1" json:"tx"`
}

func (*Transfer) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *Transfer) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeTransfer)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

// Fulfills verifies that all inputs and outputs in the transfer request are in the transfer.
// The transfer should just have new inputs and outputs added to complete any requested transfers.
func (t Transfer) Fulfills(request *TransferRequest) bool {
	for _, rtxin := range request.Tx.Tx.TxIn {
		found := false
		for _, ttxin := range t.Tx.Tx.TxIn {
			if bytes.Equal(rtxin.PreviousOutPoint.Hash[:], ttxin.PreviousOutPoint.Hash[:]) &&
				rtxin.PreviousOutPoint.Index == ttxin.PreviousOutPoint.Index &&
				rtxin.Sequence == ttxin.Sequence {
				found = true
				break
			}
		}

		if !found {
			return false
		}
	}

	for _, rtxout := range request.Tx.Tx.TxOut {
		found := false
		for _, ttxout := range t.Tx.Tx.TxOut {
			if bytes.Equal(rtxout.LockingScript[:], ttxout.LockingScript[:]) &&
				rtxout.Value == ttxout.Value {
				found = true
				break
			}
		}

		if !found {
			return false
		}
	}

	return true
}

// TransferAccept is an acceptance of a transfer. It should always be wrapped in a response to the
// transfer message. It should contain the final expanded tx if the acceptor signed any inputs or
// made any changes to the tx that effected its txid.
type TransferAccept struct {
	Tx *expanded_tx.ExpandedTx `bsor:"1" json:"tx"`
}

func (*TransferAccept) ProtocolID() envelope.ProtocolID {
	return ProtocolID
}

func (m *TransferAccept) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(Version))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(MessageTypeTransferAccept)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolID}, payload}, nil
}

// Item is something that can be included in an invoice. Commonly a product or service.
type Item struct {
	ID          bitcoin.Hex     `bsor:"1" json:"id"` // Unique identifier for the item
	Name        string          `bsor:"2" json:"name"`
	Description string          `bsor:"3" json:"description"`
	Prices      Prices          `bsor:"4" json:"prices"` // payment options to receive item
	Available   int             `bsor:"5" json:"available,omitempty"`
	Period      channels.Period `bsor:"6" json:"period"` // period of time item remains active
	Max         uint64          `bsor:"7" json:"max"`    // maximum amount for rate limited items
}

type Items []*Item

// Price is a description of the payment required. Either quantity or amount can be specified
// depending on whether the token protocol uses integers or floats to specify amounts.
type Price struct {
	Token    TokenID           `bsor:"1" json:"token"` // Token to pay with
	Quantity *uint64           `bsor:"2" json:"quantity,omitempty"`
	Amount   *channels.Decimal `bsor:"3" json:"amount,omitempty"`
}

func (p Price) Equal(other Price) bool {
	if !p.Token.Equal(other.Token) {
		return false
	}

	if (p.Quantity == nil && other.Quantity != nil) ||
		(p.Quantity != nil && other.Quantity == nil) {
		return false
	}

	if p.Quantity != nil {
		if *p.Quantity != *other.Quantity {
			return false
		}
	}

	if (p.Amount == nil && other.Amount != nil) ||
		(p.Amount != nil && other.Amount == nil) {
		return false
	}

	if p.Amount != nil {
		if !p.Amount.Equal(*other.Amount) {
			return false
		}
	}

	return true
}

type Prices []*Price

// TokenID specifies the token protocol and the unique ID of the token.
type TokenID struct {
	Protocol bitcoin.Hex `bsor:"1" json:"protocol"` // Leave empty for bitcoin
	ID       bitcoin.Hex `bsor:"2" json:"id,omitempty"`
}

func (id TokenID) IsBitcoin() bool {
	return len(id.Protocol) == 0
}

func (id TokenID) Equal(other TokenID) bool {
	if !bytes.Equal(id.Protocol, other.Protocol) {
		return false
	}

	if !bytes.Equal(id.ID, other.ID) {
		return false
	}

	return true
}

// InvoiceItem specifies an item that is being purchased. Either quantity or amount is specified but
// not both. Neither are required, for example when it is a service that is being purchased.
type InvoiceItem struct {
	ID       bitcoin.Hex       `bsor:"1" json:"id"`    // Unique identifier for the item
	Price    Price             `bsor:"2" json:"price"` // specified payment option
	Quantity *uint64           `bsor:"3" json:"quantity,omitempty"`
	Amount   *channels.Decimal `bsor:"4" json:"amount,omitempty"`
}

type InvoiceItems []*InvoiceItem

func Parse(payload envelope.Data) (channels.Message, error) {
	if len(payload.ProtocolIDs) == 0 {
		return nil, nil
	}

	if !bytes.Equal(payload.ProtocolIDs[0], ProtocolID) {
		return nil, nil
	}

	if len(payload.ProtocolIDs) != 1 {
		return nil, errors.Wrapf(channels.ErrInvalidMessage, "invoices can't wrap")
	}

	if len(payload.Payload) == 0 {
		return nil, errors.Wrapf(channels.ErrInvalidMessage, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(channels.ErrUnsupportedVersion, fmt.Sprintf("invoices %d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload.Payload[1])
	if err != nil {
		return nil, errors.Wrap(err, "message type")
	}

	result := MessageForType(MessageType(messageType))
	if result == nil {
		return nil, errors.Wrap(ErrUnsupportedInvoicesMessage,
			fmt.Sprintf("%d", MessageType(messageType)))
	}

	if _, err := bsor.Unmarshal(payload.Payload[2:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}

// Extract finds the Invoice message embedded in the tx.
func Extract(tx *wire.MsgTx) (*Invoice, error) {
	for _, txout := range tx.TxOut {
		payload, err := envelopeV1.Parse(bytes.NewReader(txout.LockingScript))
		if err != nil {
			continue
		}

		if len(payload.ProtocolIDs) != 1 ||
			!bytes.Equal(ProtocolID, payload.ProtocolIDs[0]) {
			continue
		}

		msg, err := Parse(payload)
		if err != nil {
			continue
		}

		invoice, ok := msg.(*Invoice)
		if !ok {
			continue
		}

		return invoice, nil
	}

	return nil, ErrInvoiceMissing
}

func MessageForType(messageType MessageType) channels.Message {
	switch MessageType(messageType) {
	case MessageTypeRequestMenu:
		return &RequestMenu{}
	case MessageTypeMenu:
		return &Menu{}
	case MessageTypePurchaseOrder:
		return &PurchaseOrder{}
	case MessageTypeInvoice:
		return &Invoice{}
	case MessageTypeTransferRequest:
		return &TransferRequest{}
	case MessageTypeTransfer:
		return &Transfer{}
	case MessageTypeTransferAccept:
		return &TransferAccept{}
	case MessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func MessageTypeFor(message channels.Message) MessageType {
	switch message.(type) {
	case *RequestMenu:
		return MessageTypeRequestMenu
	case *Menu:
		return MessageTypeMenu
	case *PurchaseOrder:
		return MessageTypePurchaseOrder
	case *Invoice:
		return MessageTypeInvoice
	case *TransferRequest:
		return MessageTypeTransferRequest
	case *Transfer:
		return MessageTypeTransfer
	case *TransferAccept:
		return MessageTypeTransferAccept
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
	case "request_menu":
		*v = MessageTypeRequestMenu
	case "menu":
		*v = MessageTypeMenu
	case "purchase_order":
		*v = MessageTypePurchaseOrder
	case "invoice":
		*v = MessageTypeInvoice
	case "transfer_request":
		*v = MessageTypeTransferRequest
	case "transfer":
		*v = MessageTypeTransfer
	case "accept":
		*v = MessageTypeTransferAccept
	default:
		*v = MessageTypeInvalid
		return fmt.Errorf("Unknown MessageType value \"%s\"", s)
	}

	return nil
}

func (v MessageType) String() string {
	switch v {
	case MessageTypeRequestMenu:
		return "request_menu"
	case MessageTypeMenu:
		return "menu"
	case MessageTypePurchaseOrder:
		return "purchase_order"
	case MessageTypeInvoice:
		return "invoice"
	case MessageTypeTransferRequest:
		return "transfer_request"
	case MessageTypeTransfer:
		return "transfer"
	case MessageTypeTransferAccept:
		return "accept"
	default:
		return ""
	}
}

func ResponseCodeToString(code uint32) string {
	switch code {
	case StatusTxNotAccepted:
		return "tx_not_accepted"
	case StatusInvalidOrder:
		return "invalid_order"
	case StatusUnknownItem:
		return "unknown_item"
	case StatusWrongPrice:
		return "wrong_price"
	case StatusTxNotValid:
		return "tx_not_valid"
	case StatusTxFeeTooLow:
		return "tx_fee_too_low"
	case StatusTxMissingAncestor:
		return "tx_missing_ancestor"
	case StatusTxMissingInput:
		return "tx_missing_input"
	case StatusTransferUnknown:
		return "transfer_unknown"
	case StatusMissingResponseID:
		return "missing_response_id"
	default:
		return "parse_error"
	}
}
