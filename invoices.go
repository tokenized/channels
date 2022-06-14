package channels

import (
	"bytes"
	"fmt"

	envelope "github.com/tokenized/envelope/pkg/golang/envelope/base"
	envelopeV1 "github.com/tokenized/envelope/pkg/golang/envelope/v1"
	"github.com/tokenized/pkg/bitcoin"
	"github.com/tokenized/pkg/bsor"
	"github.com/tokenized/pkg/wire"

	"github.com/pkg/errors"
)

const (
	InvoicesVersion = uint8(0)

	InvoicesMessageTypeInvalid         = InvoicesMessageType(0)
	InvoicesMessageTypeRequestMenu     = InvoicesMessageType(1)
	InvoicesMessageTypeMenu            = InvoicesMessageType(2)
	InvoicesMessageTypePurchaseOrder   = InvoicesMessageType(3)
	InvoicesMessageTypeInvoice         = InvoicesMessageType(4)
	InvoicesMessageTypeTransferRequest = InvoicesMessageType(5)
	InvoicesMessageTypeTransfer        = InvoicesMessageType(6)
	InvoicesMessageTypeTransferAccept  = InvoicesMessageType(7)

	// InvoicesStatusTxNotAccepted is a code specific to the invoices protocol that is placed
	// in a Reject message to signify that a Bitcoin transaction was not accepted by the network.
	InvoicesStatusTxNotAccepted = uint32(1)
	InvoicesStatusInvalidOrder  = uint32(2)
	InvoicesStatusUnknownItem   = uint32(3)
	InvoicesStatusWrongPrice    = uint32(4)

	// InvoicesStatusTxNotValid means the transaction or its ancestors did not validate. There
	// could be an invalid signature or parse error in the tx, or one of its ancestors.
	InvoicesStatusTxNotValid = uint32(5)

	// InvoicesStatusTxFeeTooLow means the tx does not pay the fee requirement defined in the
	// previously provided fee quote.
	InvoicesStatusTxFeeTooLow = uint32(6)

	// InvoicesStatusTxMissingAncestor means the `InvoicesRequirementAncestorsToMerkleProofs`
	// requirement was not met. Ancestors were not provided all the way to merkle proofs.
	InvoicesStatusTxMissingAncestor = uint32(7)

	// InvoicesStatusTxMissingInput means the ancestors do not meet the
	// InvoicesRequirementInputs requirement. Not all outputs being spent by the tx are included in
	// the ancestors.
	InvoicesStatusTxMissingInput = uint32(8)

	// InvoicesStatusTransferUnknown means that a received Transfer does not match any
	// previously sent TransferRequest.
	InvoicesStatusTransferUnknown = uint32(9)

	// InvoicesStatusMissingResponseID means that a message required a response id to be valid.
	InvoicesStatusMissingResponseID = uint32(10)
)

var (
	// InvoicesOptionRequireInputs specifies that all expanded txs must contain all parents. Meaning
	// the transactions containing the outputs being spent by the inputs of the expanded tx. They do
	// not require merkle proofs for them, but the tx must be included.
	InvoicesOptionRequireInputs = bitcoin.Hex{0x01}

	// InvoicesOptionRequireAncestorsToMerkleProofs specifies that all expanded txs must contain all
	// ancestors in the ancestry tree up to the nearest merkle proof. Meaning all leaves of the
	// ancestry tree must have merkle proofs.
	InvoicesOptionRequireAncestorsToMerkleProofs = bitcoin.Hex{0x02}
)

var (
	ProtocolIDInvoices = envelope.ProtocolID("I") // Protocol ID for invoice negotiation

	TokenProtocolBitcoin = []byte("Bitcoin")

	ErrUnsupportedInvoicesMessage = errors.New("Unsupported Invoices Message")
	ErrInvoiceMissing             = errors.New("Invoice Missing")
)

type InvoicesMessageType uint8

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
	return ProtocolIDRelationships
}

func (m *RequestMenu) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(InvoicesVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(InvoicesMessageTypeRequestMenu)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDInvoices}, payload}, nil
}

// Menu represents a set of items available to include in an invoice.
type Menu struct {
	Items Items `bsor:"1" json:"items"`
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
// chain communication should include a signed TransferRequest message that contains the payment tx which
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

// TransferRequest is an incomplete tx that includes an output containing the Invoice message and
// transfers of requested tokens/bitcoin for the items contained in the invoice.
type TransferRequest struct {
	Tx   *ExpandedTx     `bsor:"1" json:"tx"`
	Fees FeeRequirements `bsor:"2" json:"fees"` // tx fee requirements
}

func (*TransferRequest) ProtocolID() envelope.ProtocolID {
	return ProtocolIDInvoices
}

func (m *TransferRequest) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(InvoicesVersion))}

	// Message type
	payload = append(payload,
		bitcoin.PushNumberScriptItem(int64(InvoicesMessageTypeTransferRequest)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDInvoices}, payload}, nil
}

// Transfer is a payment transaction that embeds the approved invoice.
type Transfer struct {
	Tx *ExpandedTx `bsor:"1" json:"tx"`
}

func (*Transfer) ProtocolID() envelope.ProtocolID {
	return ProtocolIDInvoices
}

func (m *Transfer) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(InvoicesVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(InvoicesMessageTypeTransfer)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDInvoices}, payload}, nil
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
	Tx *ExpandedTx `bsor:"1" json:"tx"`
}

func (*TransferAccept) ProtocolID() envelope.ProtocolID {
	return ProtocolIDInvoices
}

func (m *TransferAccept) Write() (envelope.Data, error) {
	// Version
	payload := bitcoin.ScriptItems{bitcoin.PushNumberScriptItem(int64(InvoicesVersion))}

	// Message type
	payload = append(payload, bitcoin.PushNumberScriptItem(int64(InvoicesMessageTypeTransferAccept)))

	// Message
	msgScriptItems, err := bsor.Marshal(m)
	if err != nil {
		return envelope.Data{}, errors.Wrap(err, "marshal")
	}
	payload = append(payload, msgScriptItems...)

	return envelope.Data{envelope.ProtocolIDs{ProtocolIDInvoices}, payload}, nil
}

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
	Token    TokenID  `bsor:"1" json:"token"` // Token to pay with
	Quantity *uint64  `bsor:"2" json:"quantity,omitempty"`
	Amount   *Decimal `bsor:"3" json:"amount,omitempty"`
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

type Period struct {
	Count uint64     `json:"count"`
	Type  PeriodType `json:"type"`
}

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
	ID       bitcoin.Hex `bsor:"1" json:"id"`    // Unique identifier for the item
	Price    Price       `bsor:"2" json:"price"` // specified payment option
	Quantity *uint64     `bsor:"3" json:"quantity,omitempty"`
	Amount   *Decimal    `bsor:"4" json:"amount,omitempty"`
}

type InvoiceItems []*InvoiceItem

func ParseInvoice(payload envelope.Data) (Writer, error) {
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

func InvoicesMessageForType(messageType InvoicesMessageType) Writer {
	switch InvoicesMessageType(messageType) {
	case InvoicesMessageTypeRequestMenu:
		return &RequestMenu{}
	case InvoicesMessageTypeMenu:
		return &Menu{}
	case InvoicesMessageTypePurchaseOrder:
		return &PurchaseOrder{}
	case InvoicesMessageTypeInvoice:
		return &Invoice{}
	case InvoicesMessageTypeTransferRequest:
		return &TransferRequest{}
	case InvoicesMessageTypeTransfer:
		return &Transfer{}
	case InvoicesMessageTypeTransferAccept:
		return &TransferAccept{}
	case InvoicesMessageTypeInvalid:
		return nil
	default:
		return nil
	}
}

func InvoicesMessageTypeFor(message Writer) InvoicesMessageType {
	switch message.(type) {
	case *RequestMenu:
		return InvoicesMessageTypeRequestMenu
	case *Menu:
		return InvoicesMessageTypeMenu
	case *PurchaseOrder:
		return InvoicesMessageTypePurchaseOrder
	case *Invoice:
		return InvoicesMessageTypeInvoice
	case *TransferRequest:
		return InvoicesMessageTypeTransferRequest
	case *Transfer:
		return InvoicesMessageTypeTransfer
	case *TransferAccept:
		return InvoicesMessageTypeTransferAccept
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
	case "transfer_request":
		*v = InvoicesMessageTypeTransferRequest
	case "transfer":
		*v = InvoicesMessageTypeTransfer
	case "accept":
		*v = InvoicesMessageTypeTransferAccept
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
	case InvoicesMessageTypeTransferRequest:
		return "transfer_request"
	case InvoicesMessageTypeTransfer:
		return "transfer"
	case InvoicesMessageTypeTransferAccept:
		return "accept"
	default:
		return ""
	}
}

func InvoicesStatusToString(code uint32) string {
	switch code {
	case InvoicesStatusTxNotAccepted:
		return "tx_not_accepted"
	case InvoicesStatusInvalidOrder:
		return "invalid_order"
	case InvoicesStatusUnknownItem:
		return "unknown_item"
	case InvoicesStatusWrongPrice:
		return "wrong_price"
	case InvoicesStatusTxNotValid:
		return "tx_not_valid"
	case InvoicesStatusTxFeeTooLow:
		return "tx_fee_too_low"
	case InvoicesStatusTxMissingAncestor:
		return "tx_missing_ancestor"
	case InvoicesStatusTxMissingInput:
		return "tx_missing_input"
	case InvoicesStatusTransferUnknown:
		return "transfer_unknown"
	case InvoicesStatusMissingResponseID:
		return "missing_response_id"
	default:
		return "parse_error"
	}
}
