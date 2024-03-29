# Channels

***This software is still in early development and is prone to change in breaking ways without notice.***

Channels are a set of protocols designed to facilitate communication between users, agents, and services. Channels work on top of “peer channels”, but can work on IPv6 in the future as well. Default protocols for identification, authentication, encryption, and payment negotiation and settlement. Channels are designed to be more two directional than many existing systems like REST.

Channels facilitate relationships between parties. They are initiated by exchanging “peer channels”, each party giving a channel URL and write token to the other party, but keeping the read tokens private. So the other party can write to your channel, but only you can read from your channel. This establishes two way communication. After the relationship is established either party can initiate a new conversation. This is crucial for user to user communication, but very important for service providers as well because they will want to collect fees before the current fee is exhausted or expires. Service providers will also benefit from the ability to deliver messages and data as they come in and not need to retain that data and provide a separate interface to feed that data to the clients.

This deprecates much of paymail as users can directly exchange channels via any current communication network, but paymail can still be used to link a handle to a public relationship initiation channel. Paymail can mainly be a public method of establishing new relationships.

Channels can simplify user interactions and service interfaces. It will provide services with standardized methods of receiving payments, authentication, and data delivery. Services will no longer need to retain data and implement systems and interfaces for providing it to users. They simply define a set of message structures and post the data to the user when it is received.

Channels abstract away the two way communication to just a message delivery endpoint for each party. This simplifies the design of the sub-protocols and expands their capabilities without added complexity. With HTTP the client would have to periodically check for messages from every user and service via endpoints designed and implemented by each service, making standardization more difficult. Channels makes it now trivial for a service to periodically request payments before the current payment is exhausted or expires. The plan is to have relationships with everyone you deal with. So having nearly constant connections to all of those domains to wait for messages is not appropriate. With Channels you provide them with a method of pushing messages to you and you simply have one websocket open to see new messages come in from any relationship.

## Peer Channels

Peer channels (previously SPV channels) can be very scalable and can be directly paid for via this protocol using micro-transactions. At their base they are simply a method for delivering messages of any kind and allowing the owner of the channel to receive them without having to always be online. They are like email stripped down to the very base component and without requiring an email server to send messages for you. Peer channels are simply a message delivery system for these protocols. They are not meant to retain messages after being read. The application should handle that with a data storage service or something else. Not all messages must be saved, but signed data relating to establishing identity, invoices, and other data relevant to the supported protocols should be retained indefinitely.

The main idea is that a wallet service that can receive requests/messages won't normally have the user's private keys so can't respond with a signed message, but you can establish a relationship with a user, establish a base key for the relationship, and have all messages signed, ensuring you are directly communicating with the intended party and removing trust from the service.

If we can get most services, agents, and users on a base communication protocol that is accessible to everyone (no domain/IP required) and then have a common messaging structure, then our protocols will be easier to design.

An important feature is listening to an entire account on one websocket so if anyone posts to any of your channels you can see it instantly if you are online and listening.

### Persistent Communication

Peer channels provide persistent communication which is important for relationships that Channels are built around. Channels is designed to facilitate long term relationships. The partys identify themselves once, and build trust around that with continued interaction.

One example needing persistent communication, that I believe the original peer channels was built around, is tx confirmations. It is tedious to have to maintain a connection until a tx confirms. It is preferable to provide a method to post that data back to the sender when it happens.

Another example is user generated responses. When a user is needed to respond to a message then you can't guarantee when they are online. So the ability to post a request and provide a method to respond when the user deals with it is necessary. I believe many payments will involve several rounds of negotiation, most of which require direct user interaction on both sides.

Paymail can't get the user's approval or signature instantly. You could just send messages to a paymail and receive responses via a paymail handle and a new message posting endpoint, but that reduces privacy and I don't believe can be as distributed and dynamic as peer channels. Peer channels are cheap and efficient and each user can potentially use thousands, similar to using different bitcoin addresses to protect privacy.

One advantage over paymail is the direct user interaction. When receiving a request for a payment destination the user has the option to not provide any addresses if they don't want to interact with that party, or to provide addresses specific to the request that paymail might not know about.

Most users can't be expected to have a domain or IP so they can't provide that function themselves and need a standardized service. I believe there are many other use cases similar and even longer term than this.

## Encoding

Channels messages are wrapped in the [Envelope v1](https://github.com/tokenized/envelope) protocol to identify the data protocols used. This allows combining protocols like message signing, encryption, and different message type protocols. For example, any data protocol message can be wrapped with the message signing protocol to sign it. This doesn't require the data protocol or implementation to know anything about the signing protocol or vice versa.

Channels data protocols use Bitcoin Script Object Representations (BSOR). It is a bitcoin script encoding that supports object structures like JSON. It also uses predefined structures like Protocol Buffers and is fully binary. BSOR is important because many negotiations will end with a message being embedded in a transaction on chain, so the data should be encoded as small as possible and in a bitcoin script. Many channels may also be high throughput so decoding JSON text can be costly as opposed to reading BSOR binary data.

Since 99.99999% of the time no human directly sees the messages keeping it in a user friendly encoding is not reasonable. It is important to standardize encoding. So using JSON for off chain and BSOR for on chain is more complexity. Plus any server that wants to operate at high throughput will want to shave every second of data decode/encode time and save bandwidth. Costs of using JSON on chain will be costly at scale even with tiny fees. JSON is usually at least 3 or 4 times larger than BSOR. Just hex alone is twice as big and when you add field names that are usually 5 or more characters vs 1-2 character BSOR field ids, plus all of the escape and delimiter characters of JSON that are replaced most often by one byte push op codes in BSOR.

## Security

Channels can be used more securely than Paymail. Paymail responses are created by the server and therefore can’t be signed by the other party. A malicious paymail host can therefore swap payment locations or manipulate messages. Channel messages are signed by each party, so the only time it is susceptible to mal intent is when establishing the channel and exchanging public keys. After that messages are guaranteed to be from the expected party and can even be encrypted so the server can’t see it.

## Relationships

A relationship is a two way communication channel between two parties. A new relationship is established by exchanging peer channels, base public keys for deriving keys, and possibly identity information. Each message in a relationship should be signed by a new key derived from the user's base key.

Relationships start by establishing relevant identity. Then negotiating any payments involved like paying the first month for a service or establishing fee rates. Then a relationship can be based on conversations initiated from either side.

- A user can request data from the service.
- A service can request payment from the user.
- A user can request payment from another user.
- A user can request to pay another user.
- Users can simply send text messages to each other.

## User Communication

Users can communicate with each other to negotiate payments and just offline communication.

Payments can be negotiated in a more complex way than Paymail. Paymail simply provides payment locations. Via channels users can negotiate payments in either direction and negotiate any other terms that both parties' wallets understand.

Users are also directly involved in each step of any actions that take place, which will be important in many interactions. Most users will not want to receive blind payments. They will want to establish relationships with someone who wants to pay them for something and they will want to make it clear what is being paid for.

## Agent / Service Communication

Users can communicate with agents and service providers via channels. Agents and services can have publicly known channels to initiate new relationships. Those relationships can be continued on private channels or on the public channel.

Services provided, fees, and payment of those fees can be negotiated at the beginning of the relationship and whenever necessary. The data for those services can then be delivered directly through the channel.

Each agent and service type will need their own protocol that runs on within these protocols.

### Potential Services

#### Peer Channels

A user can connect to a peer channel service and negotiate payment for the service to create and host peer channels. The service can establish an initial relationship with the user by providing them with an initial peer channel, then use that channel to communicate payments for services providing a method of creating new channels for use with other relationships.

#### Peer Channel Posting

There can even be services that post to peer channels for you. Like if the channel goes offline for lower quality peer channel services it can continue to retry. This would be important for IPv6 where users would be offline sometimes and a service could periodically attempt to post the message until it comes back online and succeeds.

#### Data Storage

A user can negotiate a remote data service to store wallet data, like UTXOs, relationships, or any other data desired so that it won’t be lost if their device is damaged. This data can be encrypted and stored on multiple redundant services.

#### Block Chain Data

A user can connect to a block chain data service and negotiate payment for block chain data like monitoring addresses or specific on chain data like a specific data protocol, submitting txs to miners, validating incoming payments, getting headers and merkle proofs, and much more.

#### Social Network

Users can establish paid channels for distributing social media content. Users can directly subscribe to other user’s content, subscribe to curated content, or subscribe to a filter of on chain content for a specific social network protocol. This can be completely distributed and does not require any one website or point of failure to access.

## Fees

A base level protocol for specifying fees is necessary for agents and service providers. This provides for the negotiation of the invoice for what services will be provided and what payments will be required. Then the invoice is embedded in the payment tx so the service can be held accountable.

## Sample Workflows

### User to User

In user to user communication the responses may not be immediate as they often require action from
the user. If both users are online then they will see the messages instantly so the flow can seem nearly instantaneous.

#### Initiation

1. Alice shares a peer channel URL and write token with Bob via a direct message.
2. Bob posts a `Relationship Initiation` message to that channel with his public key, peer channel URL, and write token.
3. Alice responds by posting a message to Bob's peer channel with a `Relationship Accept` message containing her public key.
4. Both users verify with each other through external means to ensure they are connected to the correct person. Every transaction from here on is signed by a key derived from the shared base keys so they can be sure where the messages are coming from.

#### Usage

1. Bob posts a `Purchase Order` message to Alice's channel that says he wants to reimburse her for the lunch she just bought him.
2. Alice responds with an `Invoice` message that contains an incomplete transaction that pays Alice the correct amount.
3. Bob completes that transaction, signs it, and posts a `Payment` message containing the transaction to Alice's channel.
4. Alice signs any inputs she may have on the transaction and submits the transaction to the Bitcoin network to confirm it is valid and posts a `Payment Accept` message to Bob's peer channel.
5. When Alice receives a merkle proof for the transaction she posts a `Confirm` message containing the merkle proof to Bob's channel.

### Peer Channel Service

In user to service communication the service will often be operated by an automated agent so their responses will be immediate, but the user will need to take action on each message on their end.

#### Initiation

1. A peer channel service posts their public peer channel URL on their website.
2. Alice posts a `Relationship Initiation` message to that channel with her public key and states that she doesn't have any peer channels yet and provides a method to respond.
3. The peer channel service responds outside of peer channels with a message including a peer channel URL with a read token for Alice to use. Since Alice doesn't know the write token she can't use this channel for anything except receiving messages from the peer channel service. The peer channel service then posts a `Relationship Accept` message including a public key and a private peer channel URL and write token specific to that relationship.
4. Alice posts a `Request Menu` message to the private channel requesting the list of available services and prices.
5. The peer channel service responds with a `Menu` message that lists their available services and prices.
6. Alice chooses the services she wants, creates a `Purchase Order` message, and posts that to the peer channel.
7. The peer channel service posts an `Invoice` message that contains an incomplete transaction the pays them for the services requested.
8. Alice completes that transaction, signs it, and posts a `Payment` message containing the transaction to the peer channel.
9. The peer channel signs any inputs they may have on the transaction and submits the transaction to the Bitcoin network to confirm it is valid and posts a `Payment Accept` message to Alice's peer channel.
10. When the peer channel receives a merkle proof for the transaction they post a `Confirm` message containing the merkle proof to Alice's channel.

#### Usage

1. Alice posts a `Create Channel` message to the peer channel services private channel for their relationship.
2. The peer channel service posts a `Channel Created` message containing a new peer channel URL with read and write tokens. If channels are paid for on creation than this can be preceded by an `Invoice` message and the following procedure, or the channels can be paid on a periodic basis based on quantity, volume, or time.
3. Alice shares The peer channel URL and write token with Bob to start a new channels relationship.
