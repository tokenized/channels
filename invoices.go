package channels

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/merchant_api"
	"github.com/tokenized/pkg/merkle_proof"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

const (
	InvoicesVersion = uint8(0)

	InvoicesMessageTypeInvalid       = InvoicesMessageType(0)
	InvoicesMessageTypeRequestMenu   = InvoicesMessageType(1)
	InvoicesMessageTypeMenu          = InvoicesMessageType(2)
	InvoicesMessageTypePurchaseOrder = InvoicesMessageType(3)
	InvoicesMessageTypeInvoice       = InvoicesMessageType(4)
	InvoicesMessageTypeInvoiceTx     = InvoicesMessageType(5)
	InvoicesMessageTypePayment       = InvoicesMessageType(6)

	PeriodTypeUnspecified = PeriodType(0)
	PeriodTypeSecond      = PeriodType(1)
	PeriodTypeMinute      = PeriodType(2)
	PeriodTypeHour        = PeriodType(3)
	PeriodTypeDay         = PeriodType(4)
	PeriodTypeWeek        = PeriodType(5)
	PeriodTypeMonth       = PeriodType(6)
	PeriodTypeYear        = PeriodType(7)
)

var (
	ProtocolIDInvoices = envelope.ProtocolID("I") // Protocol ID for invoice negotiation

	ErrUnsupportedInvoicesMessage = errors.New("Unsupported Invoices Message")
	ErrInvoiceMissing             = errors.New("Invoice Missing")
)

type InvoicesMessageType uint8
type PeriodType uint8

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

func (*RequestMenu) ProtocolID() envelope.ProtocolID {
	return ProtocolIDRelationships
}

func (m *RequestMenu) Write() (envelope.ProtocolIDs, bitcoin.ScriptItems, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(InvoicesVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(InvoicesMessageTypeRequestMenu)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.ProtocolIDs{ProtocolIDInvoices}, payload, nil
}

// Menu represents a set of items available to include in an invoice.
type Menu struct {
	Items  Items     `bsor:"1" json:"items"`
	Vendor *Identity `bsor:"2" json:"vendor,omitempty"`
}

func (*Menu) ProtocolID() envelope.ProtocolID {
	return ProtocolIDInvoices
}

func (m *Menu) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(InvoicesVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(InvoicesMessageTypeMenu)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDInvoices}, payload}, nil
}

// PurchaseOrder contains items the buyer wishes to purchase.
// Identity is implicit based on the relationship and the key that signed the message.
type PurchaseOrder struct {
	Items InvoiceItems `bsor:"1" json:"items"`
	Notes *string      `bsor:"2" json:"notes,omitempty"`
}

func (*PurchaseOrder) ProtocolID() envelope.ProtocolID {
	return ProtocolIDInvoices
}

func (m *PurchaseOrder) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(InvoicesVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(InvoicesMessageTypePurchaseOrder)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDInvoices}, payload}, nil
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
	Items      InvoiceItems `bsor:"1" json:"items"`
	Notes      *string      `bsor:"2" json:"notes,omitempty"`
	Timestamp  Timestamp    `bsor:"3" json:"timestamp"`
	Expiration Timestamp    `bsor:"4" json:"expiration"`
}

func (*Invoice) ProtocolID() envelope.ProtocolID {
	return ProtocolIDInvoices
}

func (m *Invoice) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(InvoicesVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(InvoicesMessageTypeInvoice)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDInvoices}, payload}, nil
}

type Timestamp uint64 // Seconds since UNIX epoch

func Now() Timestamp {
	return Timestamp(time.Now().Unix())
}

func ConvertToTimestamp(t time.Time) Timestamp {
	return Timestamp(t.Unix())
}

// InvoiceTx is an incomplete tx that includes an output containing the InvoiceData message and
// payments for the items contained in the invoice.
type InvoiceTx struct {
	Tx ExpandedTx `bsor:"1" json:"tx"`
}

func (*InvoiceTx) ProtocolID() envelope.ProtocolID {
	return ProtocolIDInvoices
}

func (m *InvoiceTx) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(InvoicesVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(InvoicesMessageTypeInvoiceTx)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDInvoices}, payload}, nil
}

// InvoicePayment is a payment transaction that embeds the approved invoice.
type InvoicePayment struct {
	Tx   ExpandedTx             `bsor:"1" json:"tx"`
	Fees merchant_api.FeeQuotes `bsor:"3" json:"fees"` // tx fee requirements
}

func (*InvoicePayment) ProtocolID() envelope.ProtocolID {
	return ProtocolIDInvoices
}

func (m *InvoicePayment) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(InvoicesVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(InvoicesMessageTypePayment)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDInvoices}, payload}, nil
}

// ExpandedTx is a Bitcoin transaction with ancestor information.
// All ancestor transactions back to merkle proofs should be provided.
type ExpandedTx struct {
	Tx        *wire.MsgTx `bsor:"1" json:"tx"`        // marshals as raw bytes
	Outputs   Outputs     `bsor:"2" json:"outputs"`   // outputs spent by inputs of tx
	Ancestors AncestorTxs `bsor:"3" json:"ancestors"` // ancestor history of outputs up to merkle proofs
}

// Output is a Bitcoin transaction output that is spent.
type Output struct {
	Value         uint64         `bsor:"1" json:"value"`
	LockingScript bitcoin.Script `bsor:"2" json:"locking_script"`
}

type Outputs []*Output

// AncestorTx is a tx containing a spent output contained in an expanded tx or an ancestor. If it is
// confirmed then the merkle proof should be provided, otherwise the outputs and miner responses
// should be provided and their ancestors included in the expanded tx.
type AncestorTx struct {
	Tx             *wire.MsgTx               `bsor:"1" json:"tx"`                        // marshals as raw bytes
	MinerResponses []string                  `bsor:"2" json:"miner_responses,omitempty"` // signed JSON envelope responses from miners for the tx
	Outputs        Outputs                   `bsor:"3" json:"outputs,omitempty"`         // outputs spent by inputs of tx
	MerkleProof    *merkle_proof.MerkleProof `bsor:"4" json:"merkle_proof,omitempty"`
}

type AncestorTxs []*AncestorTx

// Item is something that can be included in an invoice. Commonly a product or service.
type Item struct {
	ID          bitcoin.Hex `bsor:"1" json:"id"` // Unique identifier for the item
	Name        string      `bsor:"2" json:"name"`
	Description string      `bsor:"3" json:"description"`
	Prices      Prices      `bsor:"4" json:"prices"` // payment options to receive item
	Available   int         `bsor:"5" json:"available,omitempty"`
	Period      Period      `bsor:"6" json:"period"` // period of time item remains active
	Max         uint64      `bsor:"7" json:"max"`    // maximum amount for rate limited items
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

type Period struct {
	Count uint64     `json:"count"`
	Type  PeriodType `json:"type"`
}

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

func ParseInvoice(payload envelope.Data) (ChannelsMessage, error) {
	if len(payload.ProtocolIDs) == 0 {
		return nil, nil
	}

	if !bytes.Equal(payload.ProtocolIDs[0], ProtocolIDInvoices) {
		return nil, nil
	}

	if len(payload.ProtocolIDs) != 1 {
		return nil, errors.Wrapf(ErrInvalidMessage, "invoices can't wrap")
	}

	if len(payload.Payload) == 0 {
		return nil, errors.Wrapf(ErrInvalidMessage, "payload empty")
	}

	version, err := bitcoin.ScriptNumberValue(payload.Payload[0])
	if err != nil {
		return nil, errors.Wrap(err, "version")
	}
	if version != 0 {
		return nil, errors.Wrap(ErrUnsupportedVersion, fmt.Sprintf("invoices %d", version))
	}

	messageType, err := bitcoin.ScriptNumberValue(payload.Payload[1])
	if err != nil {
		return nil, errors.Wrap(err, "message type")
	}

	result := InvoicesMessageForType(InvoicesMessageType(messageType))
	if result == nil {
		return nil, errors.Wrap(ErrUnsupportedInvoicesMessage,
			fmt.Sprintf("%d", InvoicesMessageType(messageType)))
	}

	if _, err := bsor.Unmarshal(payload.Payload[2:], result); err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}

	return result, nil
}

// ExtractInvoice finds the Invoice message embedded in the tx.
func ExtractInvoice(tx *wire.MsgTx) (*Invoice, error) {
	for _, txout := range tx.TxOut {
		payload, err := envelopeV1.Parse(bytes.NewReader(txout.LockingScript))
		if err != nil {
			continue
		}

		if len(payload.ProtocolIDs) != 1 ||
			!bytes.Equal(ProtocolIDInvoices, payload.ProtocolIDs[0]) {
			continue
		}

		msg, err := ParseInvoice(payload)
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

func InvoicesMessageForType(messageType InvoicesMessageType) ChannelsMessage {
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

func InvoicesMessageTypeFor(message ChannelsMessage) InvoicesMessageType {
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

func (v *PeriodType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for PeriodType : %d", len(data))
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v PeriodType) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v PeriodType) MarshalText() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return nil, fmt.Errorf("Unknown PeriodType value \"%d\"", uint8(v))
	}

	return []byte(s), nil
}

func (v *PeriodType) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *PeriodType) SetString(s string) error {
	switch s {
	case "second", "seconds":
		*v = PeriodTypeSecond
	case "minute", "minutes":
		*v = PeriodTypeMinute
	case "hour", "hours":
		*v = PeriodTypeHour
	case "day", "days":
		*v = PeriodTypeDay
	case "week", "weeks":
		*v = PeriodTypeWeek
	case "month", "months":
		*v = PeriodTypeMonth
	case "year", "years":
		*v = PeriodTypeYear
	default:
		*v = PeriodTypeUnspecified
		return fmt.Errorf("Unknown PeriodType value \"%s\"", s)
	}

	return nil
}

func (v PeriodType) String() string {
	switch v {
	case PeriodTypeSecond:
		return "second"
	case PeriodTypeMinute:
		return "minute"
	case PeriodTypeHour:
		return "hour"
	case PeriodTypeDay:
		return "day"
	case PeriodTypeWeek:
		return "week"
	case PeriodTypeMonth:
		return "month"
	case PeriodTypeYear:
		return "year"
	default:
		return ""
	}
}

func (v *Period) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return fmt.Errorf("Too short for Period : %d", len(data))
	}

	if string(data) == "null" {
		v.Count = 0
		v.Type = PeriodTypeUnspecified
		return nil
	}

	return v.SetString(string(data[1 : len(data)-1]))
}

func (v Period) MarshalJSON() ([]byte, error) {
	s := v.String()
	if len(s) == 0 {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", s)), nil
}

func (v Period) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

func (v *Period) UnmarshalText(text []byte) error {
	return v.SetString(string(text))
}

func (v *Period) SetString(s string) error {
	parts := strings.Split(s, " ")

	if len(parts) == 1 {
		v.Count = 1
		if err := v.Type.SetString(s); err != nil {
			return errors.Wrap(err, "type")
		}

		return nil
	}

	if len(parts) != 2 {
		return fmt.Errorf("Wrong Period spaces : got %d, want %d", len(parts), 2)
	}

	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return errors.Wrap(err, "count")
	}
	v.Count = uint64(count)

	if err := v.Type.SetString(parts[1]); err != nil {
		return errors.Wrap(err, "type")
	}

	return nil
}

func (v Period) String() string {
	if v.Count == 0 || v.Type == PeriodTypeUnspecified {
		return ""
	}
	if v.Count == 1 {
		return v.Type.String()
	}
	return fmt.Sprintf("%d %ss", v.Count, v.Type)
}

func (v Period) MarshalBinary() (data []byte, err error) {
	if v.Type == PeriodTypeUnspecified {
		return nil, nil
	}

	if v.Count == 1 {
		return []byte{byte(v.Type)}, nil
	}

	buf := &bytes.Buffer{}
	buf.WriteByte(byte(v.Type))
	if err := wire.WriteVarInt(buf, 0, v.Count); err != nil {
		return nil, errors.Wrap(err, "count")
	}

	return buf.Bytes(), nil
}

func (v *Period) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		v.Count = 0
		v.Type = PeriodTypeUnspecified
		return nil
	}

	if len(data) == 1 {
		v.Count = 1
		v.Type = PeriodType(data[0])
		return nil
	}

	if len(data) < 2 {
		v.Count = 0
		v.Type = PeriodTypeUnspecified
	}

	v.Type = PeriodType(data[0])
	count, err := wire.ReadVarInt(bytes.NewReader(data[1:]), 0)
	if err != nil {
		return errors.Wrap(err, "count")
	}
	v.Count = count
	return nil
}
