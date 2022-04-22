package channels

import (
	"bytes"
	"fmt"
	"reflect"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
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
	InvoicesMessageTypePayment       = InvoicesMessageType(5)

	ErrNotInvoice                 = errors.New("Not Invoices")
	ErrUnsupportedInvoicesVersion = errors.New("Unsupported Invoices Version")
	ErrUnsupportedInvoicesMessage = errors.New("Unsupported Invoices Message")
	ErrInvoiceMissing             = errors.New("Invoice Missing")
)

type InvoicesMessageType uint8

// Invoices provides a method for negotiating payments for products or services.
// Workflow:
//   1. Vendor sends Menu of Services/Products.
//   2. Buyer sends Purchase Order with requested Services/Products from the menu.
//   3. Vendor approves and sends the buyer an Invoice corresponding to the PurchaseOrder
//     or the vendor rejects and sends modified Invoice. This negotiation can continue indefinitely.
//   4. If the vendor approved then the buyer sends a payment that embeds the invoice otherwise the
//     buyer either quits, modifies the invoice, or keeps the same invoice and sends it back to the
//     vendor.

// Invoice is a message from the vendor representing an approved set of items to buy and the
// incomplete payment.
type Invoice struct {
	Items InvoiceItems `bsor:"1" json:"items"`
	Notes *string      `bsor:"2" json:"notes,omitempty"`
	Tx    ExpandedTx   `bsor:"3" json:"tx"`
}

// PurchaseOrder contains items the buyer wishes to purchase.
type PurchaseOrder struct {
	Items InvoiceItems `bsor:"1" json:"items"`
	Notes *string      `bsor:"2" json:"notes,omitempty"`
}

// Payment is a payment transaction that embeds the approved invoice.
type Payment struct {
	Tx ExpandedTx `bsor:"1" json:"tx"`
}

func (p *Payment) ExtractInvoice() (*Invoice, error) {
	for _, txout := range p.Tx.Tx.TxOut {
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

type ExpandedTx struct {
	Tx      *wire.MsgTx  `bsor:"1" json:"tx"`
	Outputs []wire.TxOut `bsor:"2" json:"outputs"` // outputs spent by inputs of tx
}

// RequestMenu is a request to receive the current menu.
type RequestMenu struct {
}

// Menu represents a set of items available to include in an invoice.
type Menu struct {
	Items  Items     `bsor:"1" json:"items"`
	Vendor *Identity `bsor:"2" json:"vendor,omitempty"`
}

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
	Item
	Price    Price    `bsor:"5" json:"price"` // specified payment option
	Quantity *uint64  `bsor:"6" json:"quantity,omitempty"`
	Amount   *float64 `bsor:"7" json:"amount,omitempty"`
}

type InvoiceItems []*InvoiceItem

type Identity struct {
	ID        bitcoin.Hex        `bsor:"1" json:"id"`
	PublicKey *bitcoin.PublicKey `bsor:"2" json:"public_key,omitempty"`
	Name      *string            `bsor:"3" json:"name,omitempty"`
	Email     *string            `bsor:"4" json:"email,omitempty"`
	Handle    *string            `bsor:"5" json:"handle,omitempty"`
	Phone     *string            `bsor:"6" json:"phone,omitempty"`
	Location  *Location          `bsor:"7" json:"location,omitempty"`
}

type Location struct {
	Streets    []string `bsor:"1" json:"streets"`
	City       string   `bsor:"2" json:"city"`
	Province   *string  `bsor:"3" json:"province,omitempty"` // State
	Country    *string  `bsor:"4" json:"country,omitempty"`
	PostalCode *string  `bsor:"5" json:"postal_code,omitempty"`
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
	case InvoicesMessageTypePayment:
		return &Payment{}
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
	case *Payment:
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
	case InvoicesMessageTypePayment:
		return "payment"
	default:
		return ""
	}
}

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
