# Channels

Channels are a set of protocols designed to facilitate communication between users, agents, and services. Channels work on top of “peer channels”, but can work on IPv6 in the future as well. They have built in authentication and possibly encryption.

Channels facilitate relationships between parties. They are initiated by exchanging “peer channels”, each party giving a channel URL and write token to the other party, but keeping the read tokens private. So the other party can write to your channel, but only you can read from your channel. This establishes two way communication.

This mostly deprecates paymail as users can directly exchange channels via any current communication network, but paymail can still be used to link a handle to a public relationship initiation channel.

## Encoding

Channels messages are encoded in Bitcoin Script Object Representations (BSOR). It is a bitcoin script encoding that supports object structures like JSON. It also uses predefined structures like Protocol Buffers and is fully binary.

## Security

Channels can be used more securely than Paymail. Paymail responses created by the server and therefore can’t be signed by the other party. A malicious paymail host can therefore swap payment locations or manipulate messages. Channel messages are signed by each party, so the only time it is susceptible to mal intent is when establishing the channel and exchanging public keys. After that messages are guaranteed to be from the expected party and can even be encrypted so the server can’t see it.


## User Communication

Users can communicate with each other to negotiate payments and just offline communication.

Payments can be negotiated in a more complex way than Paymail. Paymail simply provides payment locations. Via channels users can negotiate payments in either direction and negotiate any other terms that both parties' wallets understand.

Users are also directly involved in each step of any actions that take place, which will be important in many interactions. Most users will not want to receive blind payments. They will want to establish relationships with someone who wants to pay them for something and they will want to make it clear what is being paid for.

## Agent / Service Communication

Users can communicate with agents and service providers via channels. Agents and services can have publicly known channels to initiate new relationships. Those relationships can be continued on private channels or on the public channel.

Services provided, fees, and payment of those fees can be negotiated at the beginning of the relationship and whenever necessary. The data for those services can then be delivered directly through the channel.

Each agent and service type will need their own protocol that runs on top of these protocols.

## Fees

A base level protocol for specifying fees is necessary for agents and service providers. This provides for the negotiation of the invoice for what services will be provided and what payments will be required. Then the invoice is embedded in the payment tx so the service can be held accountable.
