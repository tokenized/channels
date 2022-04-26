package channels

import (
	"bytes"
	"fmt"
	"reflect"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/merchant_api"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

var (
	ProtocolIDInvoices = envelope.ProtocolID("I") // Protocol ID for invoice negotiation
	InvoicesVersion    = uint8(0)

	InvoicesMessageTypeInvalid       = InvoicesMessageType(0)
	InvoicesMessageTypeRequestMenu   = InvoicesMessageType(1)
	InvoicesMessageTypeMenu          = InvoicesMessageType(2)
	InvoicesMessageTypePurchaseOrder = InvoicesMessageType(3)
	InvoicesMessageTypeInvoice       = InvoicesMessageType(4)
	InvoicesMessageTypeInvoiceTx     = InvoicesMessageType(5)
	InvoicesMessageTypePayment       = InvoicesMessageType(6)

	ErrNotInvoice                 = errors.New("Not Invoices")
	ErrUnsupportedInvoicesVersion = errors.New("Unsupported Invoices Version")
	ErrUnsupportedInvoicesMessage = errors.New("Unsupported Invoices Message")
	ErrInvoiceMissing             = errors.New("Invoice Missing")
)

type InvoicesMessageType uint8

// Invoices provides a method for negotiating payments for products or services.
// Vendor Workflow:
//   1. Vendor sends Menu containing available Services/Products.
//   2. Buyer sends PurchaseOrder with requested Services/Products from the menu.
//   3. A. Vendor approves and sends the buyer an InvoiceTx containing an Invoice corresponding to
//      the PurchaseOrder and
//      B. Vendor rejects and sends a modified PurchaseOrder for buyer approval. This negotiation
//      can continue indefinitely.
//   4. If the vendor approved then the buyer sends a InvoicePayment that embeds the invoice
//     otherwise the buyer either quits, modifies the invoice, or keeps the same invoice and sends
//     it back to the vendor.
//
// User to User Workflow (Request To Send Payment):
//   1. User A sends either a PurchaseOrder to request to pay User B. The purchase order describes
//   the purpose of the payment.
//   2. User B responds with an InvoiceTx to specify how User A should make the payment. The
//   InvoiceTx is an incomplete tx paying User B and optionally an Invoice output describing the
//   purpose of the payment.
//   3. User A completes the transaction by adding inputs and other payment information required,
//   signs it, and responds with an InvoicePayment message.
//
// User to User Workflow (Request To Receive Payment):
//   1. User A sends an InvoiceTx to request payment from User B. The InvoiceTx contains an
//   incomplete tx paying User A and optionally an Invoice output describing the purpose of the
//   payment.
//   2. User B completes the transaction by adding inputs and other payment information required,
//   signs it, and responds with an InvoicePayment message.
//   3. User A signs any inputs they might have on the transaction and broadcasts it.

// RequestMenu is a request to receive the current menu.
type RequestMenu struct {
}

// Menu represents a set of items available to include in an invoice.
type Menu struct {
	Items  Items     `bsor:"1" json:"items"`
	Vendor *Identity `bsor:"2" json:"vendor,omitempty"`
}

// PurchaseOrder contains items the buyer wishes to purchase.
// Identity is implicit based on the relationship and the key that signed the message.
type PurchaseOrder struct {
	Items InvoiceItems `bsor:"1" json:"items"`
	Notes *string      `bsor:"2" json:"notes,omitempty"`
}

// Invoice is a message created by the vendor representing an approved set of items to buy. This is
// meant to be embedded in the payment tx so what is being paid for is recorded with the payment. It
// can be encrypted for privacy.
// Identity is implicit based on the peer channel relationship and the key that signed the message.
// The vendor can either add an input to the payment tx to sign it directly, or the buyer can retain
// signatures from the off chain communication to prove the vendor approved the payment. The off
// chain communication should include a signed InvoiceTx message that contains the payment tx which
// contains the Invoice.
type Invoice struct {
	Items InvoiceItems `bsor:"1" json:"items"`
	Notes *string      `bsor:"2" json:"notes,omitempty"`
}

// InvoiceTx is an incomplete tx that includes an output containing the InvoiceData message and
// payments for the items contained in the invoice.
type InvoiceTx struct {
	Tx ExpandedTx `bsor:"1" json:"tx"`
}

// InvoicePayment is a payment transaction that embeds the approved invoice.
type InvoicePayment struct {
	Tx   ExpandedTx             `bsor:"1" json:"tx"`
	Fees merchant_api.FeeQuotes `bsor:"3" json:"fees"` // tx fee requirements
}

// ExpandedTx is a Bitcoin transaction with ancestor information.
// All ancestor transactions back to merkle proofs should be provided.
type ExpandedTx struct {
	Tx        *wire.MsgTx `bsor:"1" json:"tx"`        // marshals as raw bytes
	Outputs   Outputs     `bsor:"2" json:"outputs"`   // outputs spent by inputs of tx
	Ancestors ParentTxs   `bsor:"3" json:"ancestors"` // ancestor history of outputs up to merkle proofs
}

// Output is a Bitcoin transaction output that is spent.
type Output struct {
	Value         uint64         `bsor:"1" json:"value"`
	LockingScript bitcoin.Script `bsor:"2" json:"locking_script"`
}

type Outputs []*Output

// ParentTx is a tx containing a spent output contained in an expanded tx or an ancestor. If it is
// confirmed then the merkle proof should be provided, otherwise the outputs should be provided and
// their ancestors included in the expanded tx.
type ParentTx struct {
	Tx          *wire.MsgTx               `bsor:"1" json:"tx"`                // marshals as raw bytes
	Outputs     Outputs                   `bsor:"2" json:"outputs,omitempty"` // outputs spent by inputs of tx
	MerkleProof *merkle_proof.MerkleProof `bsor:"3" json:"merkle_proof,omitempty"`
}

type ParentTxs []*ParentTx

// Item is something that can be included in an invoice. Commonly a product or service.
type Item struct {
	ID          bitcoin.Hex `bsor:"1" json:"id"` // Unique identifier for the item
	Name        string      `bsor:"2" json:"name"`
	Description string      `bsor:"3" json:"description"`
	Prices      Prices      `bsor:"4" json:"prices"` // payment options to receive item
	Available   int         `bsor:"5" json:"available"`
}

type Items []*Item

// Price is a description of the payment required. Either quantity or amount can be specified
// depending on whether the token protocol uses integers or floats to specify amounts.
type Price struct {
	Token    TokenID  `bsor:"1", json:"token"` // Token to pay with
	Quantity *uint64  `bsor:"2" json:"quantity,omitempty"`
	Amount   *float64 `bsor:"3" json:"amount,omitempty"`
}

type Prices []*Price

// TokenID specifies the token protocol and the unique ID of the token.
type TokenID struct {
	Protocol bitcoin.Hex `bsor:"1" json:"protocol"` // Specify Bitcoin for satoshis
	ID       bitcoin.Hex `bsor:"2" json:"id,omitempty"`
}

type InvoiceItem struct {
	ItemID          bitcoin.Hex `bsor:"1" json:"id"` // Unique identifier for the item
	ItemDescription string      `bsor:"2" json:"item_description"`
	Price           Price       `bsor:"3" json:"price"` // specified payment option
	Quantity        *uint64     `bsor:"4" json:"quantity,omitempty"`
	Amount          *float64    `bsor:"5" json:"amount,omitempty"`
}

type InvoiceItems []*InvoiceItem

func WriteInvoice(message interface{}) (envelope.ProtocolIDs, bitcoin.ScriptItems, error) {
	msgType := InvoicesMessageTypeFor(message)
	if msgType == InvoicesMessageTypeInvalid {
		return nil, nil, errors.Wrap(ErrUnsupportedInvoicesMessage,
			reflect.TypeOf(message).Name())
	}

	var scriptItems bitcoin.ScriptItems

	// Version
	scriptItems = append(scriptItems, bitcoin.PushNumberScriptItem(int64(InvoicesVersion)))

	// Message type
	scriptItems = append(scriptItems, bitcoin.PushNumberScriptItem(int64(msgType)))

	// Message
	msgScriptItems, err := bsor.Marshal(message)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal")
	}
	scriptItems = append(scriptItems, msgScriptItems...)

	return envelope.ProtocolIDs{ProtocolIDInvoices}, scriptItems, nil
}

func ParseInvoice(protocolIDs envelope.ProtocolIDs,
	payload bitcoin.ScriptItems) (interface{}, error) {

	if len(protocolIDs) != 1 {
		return nil, errors.Wrapf(ErrNotInvoice, "only one protocol supported")
	}

	if !bytes.Equal(protocolIDs[0], ProtocolIDInvoices) {
		return nil, errors.Wrapf(ErrNotInvoice, "wrong protocol id: %x", protocolIDs[0])
	}

	if len(payload) == 0 {
		return nil, errors.Wrapf(ErrNotInvoice, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(ErrUnsupportedInvoicesVersion, fmt.Sprintf("%d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload[1])
	if err != nil {
		return nil, errors.Wrap(err, "message type")
	}

	result := InvoicesMessageForType(InvoicesMessageType(messageType))
	if result == nil {
		return nil, errors.Wrap(ErrUnsupportedInvoicesMessage,
			fmt.Sprintf("%d", InvoicesMessageType(messageType)))
	}

	if _, err := bsor.Unmarshal(payload[2:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}

// ExtractInvoice finds the Invoice message embedded in the tx.
func ExtractInvoice(tx *wire.MsgTx) (*Invoice, error) {
	for _, txout := range tx.TxOut {
		protocolIDs, payload, err := envelopeV1.Parse(bytes.NewReader(txout.LockingScript))
		if err != nil {
			continue
		}

		if len(protocolIDs) != 1 || !bytes.Equal(ProtocolIDInvoices, protocolIDs[0]) {
			continue
		}

		msg, err := ParseInvoice(protocolIDs, payload)
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

func InvoicesMessageForType(messageType InvoicesMessageType) interface{} {
	switch InvoicesMessageType(messageType) {
	case InvoicesMessageTypeRequestMenu:
		return &RequestMenu{}
	case InvoicesMessageTypeMenu:
		return &Menu{}
	case InvoicesMessageTypePurchaseOrder:
		return &PurchaseOrder{}
	case InvoicesMessageTypeInvoice:
		return &Invoice{}
	case InvoicesMessageTypeInvoiceTx:
		return &InvoiceTx{}
	case InvoicesMessageTypePayment:
		return &InvoicePayment{}
	case InvoicesMessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func InvoicesMessageTypeFor(message interface{}) InvoicesMessageType {
	switch message.(type) {
	case *RequestMenu:
		return InvoicesMessageTypeRequestMenu
	case *Menu:
		return InvoicesMessageTypeMenu
	case *PurchaseOrder:
		return InvoicesMessageTypePurchaseOrder
	case *Invoice:
		return InvoicesMessageTypeInvoice
	case *InvoiceTx:
		return InvoicesMessageTypeInvoiceTx
	case *InvoicePayment:
		return InvoicesMessageTypePayment
	default:
		return InvoicesMessageTypeInvalid
	}
}

func (v *InvoicesMessageType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for InvoicesMessageType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v InvoicesMessageType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v InvoicesMessageType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown InvoicesMessageType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *InvoicesMessageType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *InvoicesMessageType) SetString(s string) error {
	switch s {
	case "request_menu":
		*v = InvoicesMessageTypeRequestMenu
	case "menu":
		*v = InvoicesMessageTypeMenu
	case "purchase_order":
		*v = InvoicesMessageTypePurchaseOrder
	case "invoice":
		*v = InvoicesMessageTypeInvoice
	case "invoice_tx":
		*v = InvoicesMessageTypeInvoiceTx
	case "payment":
		*v = InvoicesMessageTypePayment
	default:
		*v = InvoicesMessageTypeInvalid
		return fmt.Errorf("Unknown InvoicesMessageType value \"%s\"", s)
	}

	return nil
}

func (v InvoicesMessageType) String() string {
	switch v {
	case InvoicesMessageTypeRequestMenu:
		return "request_menu"
	case InvoicesMessageTypeMenu:
		return "menu"
	case InvoicesMessageTypePurchaseOrder:
		return "purchase_order"
	case InvoicesMessageTypeInvoice:
		return "invoice"
	case InvoicesMessageTypeInvoiceTx:
		return "invoice_tx"
	case InvoicesMessageTypePayment:
		return "payment"
	default:
		return ""
	}
}
