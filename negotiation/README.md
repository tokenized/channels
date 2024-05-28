# Negotiation

## Transaction Negotiation

[This](transactions.md) explains how partial transactions are structured and constructed to negotiate a final result. A negotiation is started by one party creating a partial transaction and sending it to another party via various communication channels. Then the two parties can respond back and forth to each other until a final transaction is completed and agreed to by both parties.

## Communication

Several methods of communication can be used for transaction negotiation.

### BSVAlias Negotiation

[BSVAlias](https://github.com/tokenized/pkg/blob/master/bsvalias/NegotiationTransaction.md) (Paymail) endpoints can be used to communicate the negotiation transactions. BSVAlias is not ideal for transaction negotiation because it isn't designed to be peer to peer. BSVAlias is designed to be peer to service. For example, the common endpoints allow you to communicate with the service and make a payment without the other user being involved. BSVAlias can however be used to deliver peer to peer messages, while still allowing some automated responses. This means that the endpoint doesn't return a response, but just returns an acknowledgement that the message was received. Then the response will be delivered via a callback to the initiator's BSVAlias service.

### Peer Channels

Peer channels are the preferred way to communicate peer to peer and negotiate transactions. Peer channels don't need an automated agent to automatically respond to requests like bsvalias, and the messages can be delivered directly to the user. When communicating via peer channels BSOR encoding and [these](negotiation.go) structures are used.
