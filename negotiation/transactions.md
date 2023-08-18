
# Transaction Negotiation

*This document is an explanation of the transaction negotiation process and doesn't cover the specific message structures.*

Every transaction involving more than one party involves some negotiation between the parties. Even in the simplest scenario on party has to provide information about which locking scripts to pay to.

Peer to peer negotiation means that the parties are actually sending and responding to the messages. If a service or agent is responding on behalf of a party then it is not peer to peer. In a true peer to peer wallet you have control of who you receive payments from by not providing outputs to anyone you don't want to pay you.

Transaction negotiation involves creating a partial transaction and sending it to the other party/parties, then they make modifications to the transaction and send it back, then repeat until both parties agree. Normally this only involves 3 to 4 steps but can be more if the actual terms of the interaction are being changed and negotiated.

## Warnings

Some of the steps can be automated, but at each step the transaction should be analyzed to determine if the terms of the negotiation have changed and the user made aware if it has. For example, if the initiator offers a token send to the counterparty and the counterparty returns a transaction where they are now taking all the bitcoin change, rather than the initiator's original change outputs, than the initiator should at the very least be notified, but the default should probably be to reject the transaction and not sign it. The same is true for token change and amounts. It is important to ensure the other party doesn't modify parts of the transaction that they are not meant to.

## Techniques

### Masked Input

Masked inputs are used to maintain privacy during a negotiation. They can be used by the wallet service for automated responses to retain privacy until the user approves the request. The wallet shouldn't give out input/UTXO data without the user's permission.

When providing a masked input the other party may still need to know certain things about the intended input. They will likely need to know the value of the UTXO being spent and estimated size of the unlocking script that will be required. This data allows the other party to know how much bitcoin is being transacted and calculate the transaction mining fee.

This [unlocking script data structure](https://github.com/tokenized/txbuilder/blob/master/unlocking_data/unlocking_data.go) can provide that data in the unlocking script. It can be recognized by the fact that it is an OP_FALSE OP_RETURN script in the unlocking script of the input, which is never valid in a final transaction.

It provides these fields.

* Size - The estimated size of the final unlocking script.
* Value - The value, in satoshis, of the output to be spent.
* Party - An indicator of which party this input belongs to. 0 is the initiator.

### Masked Output

Masked outputs are used to maintain privacy during a negotiation. They can be used in requests to remove remainder bitcoin from the equation without giving out an actual locking script.

The script `OP_TRUE OP_RETURN` can be used so that it is easily identifiable as not a real output since it is spendable by anyone and would not likely be used in reality.

## Basic Scenarios

There are standard scenarios that can help explain how this works. After each step the new state of the transaction is sent to the other party.

* Initiator (I) - The party that sends the first message to initiate the negotiation.
* Counterparty (C) - The party that is on the other side of the negotiation from the initiator.
* Instruments - Bitcoin or tokens.

### Send

Send is when the initiating party wants to send instruments to the counterparty.

* (I) Create a partial transaction containing more sent instruments than received.
* (C) Add destinations for the excess instruments.
* (I) Verify and sign.

### Receive

Receive is when the initiating party wants to receive instruments from the counterparty.

* (I) Create a partial transaction containing more received instruments than sent.
* (C) Add inputs and senders of the instruments needed to equal those received then sign the transaction.
* (I) Verify and accept.

### Exchange

Exchange is when the initiating party wants to exchange some instruments for other instruments with the counterparty.

* (I) Create a partial transaction containing more received instruments than sent for one or more instruments and more sent instruments than received for other instruments.
* (C) Add inputs and senders of the instruments needed to equal those received, then add destinations for those instruments with excess senders, then sign the transaction. At this point the transaction will have some inputs signed by the counterparty, but other inputs that have yet to be signed by the initiator because the initiator could not have signed before the counterparty made their updates
* (I) Verify and sign their inputs.


### "2 Step" Exchange

The UX of an exchange can be improved if the counterparty automatically shares some information about their wallet.

* (I) Create a partial transaction containing more received instruments than sent for one or more instruments and more sent instruments than received for other instruments.
* (C) The counterparty's wallet service automatically adds masked inputs and senders of the instruments needed to equal those received, then add destinations for those instruments with excess senders, but does not sign the transaction. Masked inputs use a zero hash and zero index in the outpoint to retain privacy until the user approves the interaction. This way only specific quantities of instruments are shared.
* (I) The initiator can now sign their inputs with the signature hash flag ANYONE_CAN_PAY. This locks in all of the output data so the counterparty can't modify what is sent or received, but allows the counterparty to update their inputs before signing.
* (C) Now the counterparty can update their inputs with actual outpoint hashes and indexes, then sign the tx.
* (I) Verify and accept.

Since the first response from the counterparty is automated the initiator can perform the first two steps as one as far as the UX is concerned, with a quick request/response in the middle. Then the second UX step is simply for the counterparty to sign.

The counterparty must provide specific input values and send token quantities, so there is some loss of privacy, but the counterparty can obfuscate that by splitting those UTXOs and token quantities or by just not spending all of the tokens from one locking script. They can't change any of those quantities after the initiator has signed the transaction so if they don't reserve those items and spend them, then they might have to use preparation transactions to recreate those values with new UTXOs.

## Examples

### Send Bitcoin

Create partial transaction with more bitcoin sent than received.

The transaction mining fee should be zero so the counterparty knows exactly how many satoshis they are expected to receive. This can be done by providing a masked output to maintain privacy during the negotiation. Inputs can also be masked if desired.

The expanded tx JSON would look like this:

```
{
  "tx": "0100000001b66ed8f3d01d4d63102b90ea64671494ba03597b89eff5b87dcbc25d8cc8a6bd0000000000ffffffff01d85900000000000002516a00000000",
  "ancestors": [
    {
      "tx": "0100000001cbe15d3e05b2cd25af2ffb6e2c6ae99d230d12fa684b0b59b2f03fb033eef48b020000006b483045022100a14b7788a2af20d165b222a53161330a2480ae3fe4052e320b966d40b5fe6c6202204d7b5fdc41f80572657d0e92ae4d6efc269333d3a173e89e75943b8ceca3d7cd412103a3f806bdb045f9be629ebbbee563a512b7dfb544d0b86bed6c6690cbaa4a308bffffffff0278e00100000000001976a914a43b489965b6b360395d2593f099754c33812ec688acdc030000000000001976a9147ffccbfc4e46bcdded9ec1091f4e4b57db80e84588ac00000000"
    }
  ]
}
```

Here is a text representation:

```
TxId: 7dfd732ba7abba91fbf7927f284a13097b1bd7ee49f16964bb163bf4cc3becb8 (62 bytes)
  Version: 1
  Inputs:

    Outpoint: 0 - bda6c88c5dc2cb7db8f5ef897b5903ba94146764ea902b10634d1dd0f3d86eb6
    Script:
    Sequence: ffffffff

  Outputs:

    Value: 0.00023000
    Script: OP_1 OP_RETURN

  LockTime: 0

Fee: 100000 (1612.903198 sat/byte)
Ancestors: 1
  TxId: bda6c88c5dc2cb7db8f5ef897b5903ba94146764ea902b10634d1dd0f3d86eb6 (226 bytes)
  Version: 1
  Inputs:

    Outpoint: 2 - 8bf4ee33b03ff0b2590b4b68fa120d239de96a2c6efb2faf25cdb2053e5de1cb
    Script: 0x3045022100a14b7788a2af20d165b222a53161330a2480ae3fe4052e320b966d40b5fe6c6202204d7b5fdc41f80572657d0e92ae4d6efc269333d3a173e89e75943b8ceca3d7cd41 0x03a3f806bdb045f9be629ebbbee563a512b7dfb544d0b86bed6c6690cbaa4a308b
    Sequence: ffffffff

  Outputs:

    Value: 0.00123000
    Script: OP_DUP OP_HASH160 0xa43b489965b6b360395d2593f099754c33812ec6 OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00000988
    Script: OP_DUP OP_HASH160 0x7ffccbfc4e46bcdded9ec1091f4e4b57db80e845 OP_EQUALVERIFY OP_CHECKSIG

  LockTime: 0
  0 Miner Responses
```

This transaction has one input and one output. The input references the first output from a transaction in the ancestors that has an output value of 123,000 satoshis. The output has a value of 23,000 satoshis. The output is also masked as you can see the script is `OP_1 OP_RETURN`.

This transaction is a request to send 100,000 satoshis to the counterparty. The recipient of this message should add outputs totaling to 100,000 satoshis. There can be several outputs, but too many could be considered abuse.

An appropriate response would look like this:

```
{
  "tx": "0100000001b66ed8f3d01d4d63102b90ea64671494ba03597b89eff5b87dcbc25d8cc8a6bd0000000000ffffffff02d85900000000000002516aa0860100000000001976a91430c6c5b7e7fd4739188a62492f1fdbebffe156d588ac00000000",
  "ancestors": [
    {
      "tx": "0100000001cbe15d3e05b2cd25af2ffb6e2c6ae99d230d12fa684b0b59b2f03fb033eef48b020000006b483045022100a14b7788a2af20d165b222a53161330a2480ae3fe4052e320b966d40b5fe6c6202204d7b5fdc41f80572657d0e92ae4d6efc269333d3a173e89e75943b8ceca3d7cd412103a3f806bdb045f9be629ebbbee563a512b7dfb544d0b86bed6c6690cbaa4a308bffffffff0278e00100000000001976a914a43b489965b6b360395d2593f099754c33812ec688acdc030000000000001976a9147ffccbfc4e46bcdded9ec1091f4e4b57db80e84588ac00000000"
    }
  ]
}
```

Here is a text representation:

```
TxId: 844076025be81a934276680428a06718d9279289a8245232ec9aab4182a3849f (96 bytes)
  Version: 1
  Inputs:

    Outpoint: 0 - bda6c88c5dc2cb7db8f5ef897b5903ba94146764ea902b10634d1dd0f3d86eb6
    Script:
    Sequence: ffffffff

  Outputs:

    Value: 0.00023000
    Script: OP_1 OP_RETURN

    Value: 0.00100000
    Script: OP_DUP OP_HASH160 0x30c6c5b7e7fd4739188a62492f1fdbebffe156d5 OP_EQUALVERIFY OP_CHECKSIG

  LockTime: 0

Fee: 0 (0.000000 sat/byte)
Ancestors: 1
  TxId: bda6c88c5dc2cb7db8f5ef897b5903ba94146764ea902b10634d1dd0f3d86eb6 (226 bytes)
  Version: 1
  Inputs:

    Outpoint: 2 - 8bf4ee33b03ff0b2590b4b68fa120d239de96a2c6efb2faf25cdb2053e5de1cb
    Script: 0x3045022100a14b7788a2af20d165b222a53161330a2480ae3fe4052e320b966d40b5fe6c6202204d7b5fdc41f80572657d0e92ae4d6efc269333d3a173e89e75943b8ceca3d7cd41 0x03a3f806bdb045f9be629ebbbee563a512b7dfb544d0b86bed6c6690cbaa4a308b
    Sequence: ffffffff

  Outputs:

    Value: 0.00123000
    Script: OP_DUP OP_HASH160 0xa43b489965b6b360395d2593f099754c33812ec6 OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00000988
    Script: OP_DUP OP_HASH160 0x7ffccbfc4e46bcdded9ec1091f4e4b57db80e845 OP_EQUALVERIFY OP_CHECKSIG

  LockTime: 0
  0 Miner Responses
```

The initiator then removes the masked output and adds any new outputs necessary for change so the mining fee is correct, sign the input, then return the transaction to the counterparty.

The counterparty should then verify the transaction, return a positive acknowledge to the initiator, and post the transaction to the bitcoin network.

If there is something that the counterparty doesn't approve of then they should send a negative acknowledge and both parties should drop the transaction and not broadcast it.

When the initiator receives a positive acknowledge then they can also broadcast the transaction to the bitcoin network.

When either party receives a merkle proof they should send it to the other party.


### Receive Bitcoin

Create partial transaction with more bitcoin received than sent.

The transaction mining fee should be zero so the counterparty knows exactly how many satoshis they are expected to send.

The expanded tx JSON would look like this:
```
{
	"tx": "010000000003cb530000000000001976a914a8eef87647159008c20695d8e0eb7b6e4bc9735a88acbf0b0100000000001976a9140e91abdff0187ba9a23517eece871648da62634888ac16270000000000001976a9144d8db1765055e7e0b60ef919e2c40deb9cb5f9a388ac00000000"
}
```

Here is a text representation:

```
TxId: fac4f658d998563f207240f856bc0b45b29ba4d6d83ea5c77668f95aae3947bf (112 bytes)
  Version: 1
  Inputs:

  Outputs:

    Value: 0.00021451
    Script: OP_DUP OP_HASH160 0xa8eef87647159008c20695d8e0eb7b6e4bc9735a OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00068543
    Script: OP_DUP OP_HASH160 0x0e91abdff0187ba9a23517eece871648da626348 OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00010006
    Script: OP_DUP OP_HASH160 0x4d8db1765055e7e0b60ef919e2c40deb9cb5f9a3 OP_EQUALVERIFY OP_CHECKSIG

  LockTime: 0

Fee: Negative Fee
Ancestors: 0
```

The transaction has no inputs but three outputs that add up to 100,000 satoshis.

The transaction is a request to receive 100,000 satoshis from the counterparty. The recipient of this message should add inputs totaling more than the amount required as well as the associated ancestor transactions. Then add outputs for change to adjust to the appropriate mining fee, then sign the transaction and send it back to the initiator.

An appropriate response would look like this:

```
{
  "tx": "0100000001fac5630f81cc3b2a530784bdfa7387d255a491ab4c0a75691cc763159a3d6cb7000000006b483045022100f46dc94c85a6326ec13459c41b35e39261150d09fb0a35287b1a19a02e5a686102207b75abe41545baa299e155f0ab0dac147aeea010f8c65c2b8173d2dd61d66725412103019a9a8ecad839318a2d2295944d988d7e6b741b4735db69861785e49fc783b5ffffffff04cb530000000000001976a914a8eef87647159008c20695d8e0eb7b6e4bc9735a88acbf0b0100000000001976a9140e91abdff0187ba9a23517eece871648da62634888ac16270000000000001976a9144d8db1765055e7e0b60ef919e2c40deb9cb5f9a388acd12e0000000000001976a9149df0ac26def6ad4dc5e7a9be0ad316d1179303e888ac00000000",
  "ancestors": [
    {
      "tx": "01000000000180b50100000000001976a91440d9699b45fa5b5d28f0bc13e917536ce50e94b988ac00000000"
    }
  ]
}
```

Here is a text representation:

```
TxId: 3e8b78c4b7e4093c2ec8d7bb3749a6a4ff6dff50f979df998f1222eafd36e828 (294 bytes)
  Version: 1
  Inputs:

    Outpoint: 0 - b76c3d9a1563c71c69750a4cab91a455d28773fabd8407532a3bcc810f63c5fa
    Script: 0x3045022100f46dc94c85a6326ec13459c41b35e39261150d09fb0a35287b1a19a02e5a686102207b75abe41545baa299e155f0ab0dac147aeea010f8c65c2b8173d2dd61d6672541 0x03019a9a8ecad839318a2d2295944d988d7e6b741b4735db69861785e49fc783b5
    Sequence: ffffffff

  Outputs:

    Value: 0.00021451
    Script: OP_DUP OP_HASH160 0xa8eef87647159008c20695d8e0eb7b6e4bc9735a OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00068543
    Script: OP_DUP OP_HASH160 0x0e91abdff0187ba9a23517eece871648da626348 OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00010006
    Script: OP_DUP OP_HASH160 0x4d8db1765055e7e0b60ef919e2c40deb9cb5f9a3 OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00011985
    Script: OP_DUP OP_HASH160 0x9df0ac26def6ad4dc5e7a9be0ad316d1179303e8 OP_EQUALVERIFY OP_CHECKSIG

  LockTime: 0

Fee: 15 (0.051020 sat/byte)
Ancestors: 1
  TxId: b76c3d9a1563c71c69750a4cab91a455d28773fabd8407532a3bcc810f63c5fa (44 bytes)
  Version: 1
  Inputs:

  Outputs:

    Value: 0.00112000
    Script: OP_DUP OP_HASH160 0x40d9699b45fa5b5d28f0bc13e917536ce50e94b9 OP_EQUALVERIFY OP_CHECKSIG

  LockTime: 0
  0 Miner Responses
```

The initiator should then verify the transaction, return a positive acknowledge to the counterparty, and post the transaction to the bitcoin network.

If there is something that the initiator doesn't approve of then they should send a negative acknowledge and both parties should drop the transaction and not broadcast it.

When the counterparty receives a positive acknowledge then they can also broadcast the transaction to the bitcoin network.

When either party receives a merkle proof they should send it to the other party.


### Send Tokens

Create partial transaction with more tokens sent than received.

The transaction mining fee should be zero so the counterparty knows that no bitcoin is being exchanged. This can be done by providing a masked output to maintain privacy during the negotiation. Inputs can also be masked if desired.

The expanded tx JSON would look like this:

```
{
  "tx": "01000000029a209f69f58cf59a9b4bb3cec95c981ea6e2ad6d8bddee2b7964997113baf84c0000000000ffffffff00000000000000000000000000000000000000000000000000000000000000000000000015006a02bd015102554c585153510295005201785351ffffffff0279000000000000001976a91439ac503b1cd334d07f49698d999755c698d1c6ff88ac00000000000000005e006a02bd015108746573742e544b4e530100025431480a3c12034343591a1497375fff8feb91fe4fd77cac0702cac6cb6d41ff220310d00f2a1a0a1520ef2294e0df3cacd5df00e77cb78ee1e975c4f03310dc0b10e5c390dadf9eb9be1700000000",
  "ancestors": [
    {
      "tx": "0100000001ce21a276d62ae97d8d9b192f77ca0a208d9d8534d029cb71a455c685814f82b2060000006b483045022100ca76106dd3226b6dc1fea7d97089f26aa7897d0561581ac054fc419294c8d95602201c9c99b1da7b61b4e9d9bce43da7c239371ee3c3d9008fbe4231eb4575986e60412103a26aaa03e10133ccce04e38c90daab26b50c3bea607d6f3fb5957167e2a2ef02ffffffff0201000000000000001976a914d3b0fa70ce0a0bc63e1a5b4ef563c0850f9c630f88acdc030000000000001976a914cf94c44ff83a59fa350fff673fceb91b7ebaa17b88ac00000000"
    }
  ]
}
```

Here is a text representation:

```
TxId: 48af727f0aed72ad9a3f4c2343fc98840b2cc2507ab3de1f0636f26f7368a895 (250 bytes)
  Version: 1
  Inputs:

    Outpoint: 0 - 4cf8ba13719964792beedd8b6dade2a61e985cc9ceb34b9b9af58cf5699f209a
    Script:
    Sequence: ffffffff

    Outpoint: 0 - 0000000000000000000000000000000000000000000000000000000000000000
    Script: OP_0 OP_RETURN 445 OP_1 "UL" OP_8 OP_1 OP_3 OP_1 149 OP_2 120 OP_3 OP_1
    Sequence: ffffffff

  Outputs:

    Value: 0.00000121
    Script: OP_DUP OP_HASH160 0x39ac503b1cd334d07f49698d999755c698d1c6ff OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00000000
    Script: OP_0 OP_RETURN 445 OP_1 0x746573742e544b4e OP_3 0 "T1" 0x0a3c12034343591a1497375fff8feb91fe4fd77cac0702cac6cb6d41ff220310d00f2a1a0a1520ef2294e0df3cacd5df00e77cb78ee1e975c4f03310dc0b10e5c390dadf9eb9be17

  LockTime: 0

Fee: input output 1: parent:0000000000000000000000000000000000000000000000000000000000000000: Missing Input
Ancestors: 1
  TxId: 4cf8ba13719964792beedd8b6dade2a61e985cc9ceb34b9b9af58cf5699f209a (226 bytes)
  Version: 1
  Inputs:

    Outpoint: 6 - b2824f8185c655a471cb29d034859d8d200aca772f199b8d7de92ad676a221ce
    Script: 0x3045022100ca76106dd3226b6dc1fea7d97089f26aa7897d0561581ac054fc419294c8d95602201c9c99b1da7b61b4e9d9bce43da7c239371ee3c3d9008fbe4231eb4575986e6041 0x03a26aaa03e10133ccce04e38c90daab26b50c3bea607d6f3fb5957167e2a2ef02
    Sequence: ffffffff

  Outputs:

    Value: 0.00000001
    Script: OP_DUP OP_HASH160 0xd3b0fa70ce0a0bc63e1a5b4ef563c0850f9c630f OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00000988
    Script: OP_DUP OP_HASH160 0xcf94c44ff83a59fa350fff673fceb91b7ebaa17b OP_EQUALVERIFY OP_CHECKSIG

  LockTime: 0
  0 Miner Responses

Tokenized Test Action:
Transfer {
  "Instruments": [
    {
      "InstrumentType": "CCY",
      "InstrumentCode": "lzdf/4/rkf5P13ysBwLKxsttQf8=",
      "InstrumentSenders": [
        {
          "Quantity": 2000
        }
      ],
      "InstrumentReceivers": [
        {
          "Address": "IO8ilODfPKzV3wDnfLeO4el1xPAz",
          "Quantity": 1500
        }
      ]
    }
  ],
  "OfferExpiry": 1692479305118130661
}
Instrument ID 0: CCYEnZNVKuxLPvWZasP82jkrcGmiTbs6TYE1
```

The first input references the first output from a transaction in the ancestors that has an output value of 1 satoshi. It is just a dust UTXO to authorized the token send with the smart contract agent.

The second input is masked but specifies the unlock size of 149 and an output value of 120.

The first output is to the smart contract agent locking script and contains the contract fee and response funding totaling 121 satoshis.

The second output is the OP_RETURN and contains the T1 Transfer Tokenized request.

Since the input total of 121 equals the output total value of 121 the counterparty knows that no bitcoin is to be transferred as a result of this negotiation.

The transfer action shows a sender of 2000 tokens and a receiver of 1500 tokens. This is a request to send 500 tokens to the counterparty since there are 500 more sent tokens than received. The 1500 is just change.

The recipient of this message should update the T1 Transfer Tokenized request output to add receivers totaling 500 tokens.

An appropriate response would look like this:

```
{
  "tx": "01000000029a209f69f58cf59a9b4bb3cec95c981ea6e2ad6d8bddee2b7964997113baf84c0000000000ffffffff00000000000000000000000000000000000000000000000000000000000000000000000015006a02bd015102554c585153510295005201785351ffffffff0279000000000000001976a91439ac503b1cd334d07f49698d999755c698d1c6ff88ac00000000000000007b006a02bd015108746573742e544b4e5301000254314c640a5812034343591a1497375fff8feb91fe4fd77cac0702cac6cb6d41ff220310d00f2a1a0a1520ef2294e0df3cacd5df00e77cb78ee1e975c4f03310dc0b2a1a0a15202e32b63891294e6e3df6a954d1f85f9d41cbceeb10f40310e5c390dadf9eb9be1700000000",
  "ancestors": [
    {
      "tx": "0100000001ce21a276d62ae97d8d9b192f77ca0a208d9d8534d029cb71a455c685814f82b2060000006b483045022100ca76106dd3226b6dc1fea7d97089f26aa7897d0561581ac054fc419294c8d95602201c9c99b1da7b61b4e9d9bce43da7c239371ee3c3d9008fbe4231eb4575986e60412103a26aaa03e10133ccce04e38c90daab26b50c3bea607d6f3fb5957167e2a2ef02ffffffff0201000000000000001976a914d3b0fa70ce0a0bc63e1a5b4ef563c0850f9c630f88acdc030000000000001976a914cf94c44ff83a59fa350fff673fceb91b7ebaa17b88ac00000000"
    }
  ]
}
```

Here is a text representation:

```
TxId: 205b6e44c39c57ec58c4ec80a3010a13a680f1738fddf0ce74d5a54ae6c987d0 (279 bytes)
  Version: 1
  Inputs:

    Outpoint: 0 - 4cf8ba13719964792beedd8b6dade2a61e985cc9ceb34b9b9af58cf5699f209a
    Script:
    Sequence: ffffffff

    Outpoint: 0 - 0000000000000000000000000000000000000000000000000000000000000000
    Script: OP_0 OP_RETURN 445 OP_1 "UL" OP_8 OP_1 OP_3 OP_1 149 OP_2 120 OP_3 OP_1
    Sequence: ffffffff

  Outputs:

    Value: 0.00000121
    Script: OP_DUP OP_HASH160 0x39ac503b1cd334d07f49698d999755c698d1c6ff OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00000000
    Script: OP_0 OP_RETURN 445 OP_1 0x746573742e544b4e OP_3 0 "T1" 0x0a5812034343591a1497375fff8feb91fe4fd77cac0702cac6cb6d41ff220310d00f2a1a0a1520ef2294e0df3cacd5df00e77cb78ee1e975c4f03310dc0b2a1a0a15202e32b63891294e6e3df6a954d1f85f9d41cbceeb10f40310e5c390dadf9eb9be17

  LockTime: 0

Fee: input output 1: parent:0000000000000000000000000000000000000000000000000000000000000000: Missing Input
Ancestors: 1
  TxId: 4cf8ba13719964792beedd8b6dade2a61e985cc9ceb34b9b9af58cf5699f209a (226 bytes)
  Version: 1
  Inputs:

    Outpoint: 6 - b2824f8185c655a471cb29d034859d8d200aca772f199b8d7de92ad676a221ce
    Script: 0x3045022100ca76106dd3226b6dc1fea7d97089f26aa7897d0561581ac054fc419294c8d95602201c9c99b1da7b61b4e9d9bce43da7c239371ee3c3d9008fbe4231eb4575986e6041 0x03a26aaa03e10133ccce04e38c90daab26b50c3bea607d6f3fb5957167e2a2ef02
    Sequence: ffffffff

  Outputs:

    Value: 0.00000001
    Script: OP_DUP OP_HASH160 0xd3b0fa70ce0a0bc63e1a5b4ef563c0850f9c630f OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00000988
    Script: OP_DUP OP_HASH160 0xcf94c44ff83a59fa350fff673fceb91b7ebaa17b OP_EQUALVERIFY OP_CHECKSIG

  LockTime: 0
  0 Miner Responses

Tokenized Test Action:
Transfer {
  "Instruments": [
    {
      "InstrumentType": "CCY",
      "InstrumentCode": "lzdf/4/rkf5P13ysBwLKxsttQf8=",
      "InstrumentSenders": [
        {
          "Quantity": 2000
        }
      ],
      "InstrumentReceivers": [
        {
          "Address": "IO8ilODfPKzV3wDnfLeO4el1xPAz",
          "Quantity": 1500
        },
        {
          "Address": "IC4ytjiRKU5uPfapVNH4X51By87r",
          "Quantity": 500
        }
      ]
    }
  ],
  "OfferExpiry": 1692479305118130661
}
Instrument ID 0: CCYEnZNVKuxLPvWZasP82jkrcGmiTbs6TYE1
```

The initiator then updates the masked input, re-calculates the smart contract agent response fee and updates the smart contract agent output value, adds appropriate change outputs, and signs the inputs. Then sends the final tx to the counterparty.

The counterparty should then verify the transaction, return a positive acknowledge to the initiator, and post the transaction to the smart contract agent.

If there is something that the counterparty doesn't approve of then they should send a negative acknowledge and both parties should drop the transaction and not broadcast it.

When the initiator receives a positive acknowledge then they can also broadcast the transaction to the smart contract agent.

The smart contract agent will reply with a response transaction and when merkle proofs are available it will post those on the reply to channel as well.

When either party receives a response transaction from the smart contract agent or merkle proofs they should send them to the other party.


### Receive Tokens

Create partial transaction with more tokens received than sent.

The transaction mining fee should be zero so the counterparty knows that no bitcoin is being exchanged. This can be done by providing a masked output to maintain privacy during the negotiation. Inputs can also be masked if desired.

The expanded tx JSON would look like this:

```
{
  "tx": "010000000100000000000000000000000000000000000000000000000000000000000000000000000015006a02bd015102554c585153510295005201755351ffffffff0275000000000000001976a9143f12d3b8fa696d3b09b61b3a8e6dffc1582f357f88ac000000000000000059006a02bd015108746573742e544b4e530100025431430a3712034343591a14ffefd01ec5f7e98020e76e16b8470d13016b610d2a1a0a152095230ba1a58da05ae1f978e531d5dd147920be9110904e10a4cc9197c9c9eebe1700000000"
}
```

Here is a text representation:

```
TxId: a5cd94504c04ec663a624ef390f4aa42b263123cbda77131966f9d2625eb7e0b (204 bytes)
  Version: 1
  Inputs:

    Outpoint: 0 - 0000000000000000000000000000000000000000000000000000000000000000
    Script: OP_0 OP_RETURN 445 OP_1 "UL" OP_8 OP_1 OP_3 OP_1 149 OP_2 117 OP_3 OP_1
    Sequence: ffffffff

  Outputs:

    Value: 0.00000117
    Script: OP_DUP OP_HASH160 0x3f12d3b8fa696d3b09b61b3a8e6dffc1582f357f OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00000000
    Script: OP_0 OP_RETURN 445 OP_1 0x746573742e544b4e OP_3 0 "T1" 0x0a3712034343591a14ffefd01ec5f7e98020e76e16b8470d13016b610d2a1a0a152095230ba1a58da05ae1f978e531d5dd147920be9110904e10a4cc9197c9c9eebe17

  LockTime: 0

Fee: input output 0: parent:0000000000000000000000000000000000000000000000000000000000000000: Missing Input
Ancestors: 0

Tokenized Test Action
Transfer {
  "Instruments": [
    {
      "InstrumentType": "CCY",
      "InstrumentCode": "/+/QHsX36YAg524WuEcNEwFrYQ0=",
      "InstrumentReceivers": [
        {
          "Address": "IJUjC6GljaBa4fl45THV3RR5IL6R",
          "Quantity": 10000
        }
      ]
    }
  ],
  "OfferExpiry": 1692713873005897252
}
Instrument ID 0: CCYQLGbR1gVBKeH3axYsPSsaMZhrRh8i3xGU
```

The first input is masked but specifies the unlock size of 149 and an output value of 117.

The first output is to the smart contract agent locking script and contains the contract fee and response funding totaling 117 satoshis.

The second output is the OP_RETURN and contains the T1 Transfer Tokenized request.

Since the input total of 117 equals the output total value of 117 the counterparty knows that no bitcoin is to be transferred as a result of this negotiation.

The transfer action shows no senders of tokens and a receiver of 10000 tokens. This is a request to receive 10000 tokens from the counterparty.

The recipient of this message should update the T1 Transfer Tokenized request output to add senders totaling at least 10000 tokens and any change receivers that are necessary. This also involves adding inputs to the transaction to authorize the sends with the smart contract agent and also possibly change outputs if there are more than dust satoshis in the inputs. These inputs should not be masked as the transaction needs to be ready to sign. The recipient should then re-calculate the smart contract agent output amount for the response mining fee and adjust the transaction mining fee. Then the recipient should sign the transaction and send it back to the initiator for approval.

An appropriate response would look like this:

```
{
  "tx": "01000000019f9961459d0d408d2cdeb38b47ee3e8b832749d3038f54d4a640a800c4c9145b000000006b483045022100ad0884e26fbb53e50ec8f0b44c2c73ab934aa24d2088706dc9d0f364d9e9abce02204c201c6d430d3f08532bc47e62c734ec7452f2e884ebe698a545e94721db6bd44121029a7039e014d2dc5cb9bd10b07ab1a73773ae70e364d8691dcefa791a28ee3cccffffffff0379000000000000001976a9143f12d3b8fa696d3b09b61b3a8e6dffc1582f357f88ac00000000000000005e006a02bd015108746573742e544b4e530100025431480a3c12034343591a14ffefd01ec5f7e98020e76e16b8470d13016b610d220310904e2a1a0a152095230ba1a58da05ae1f978e531d5dd147920be9110904e10a4cc9197c9c9eebe1766550000000000001976a91438daf933e7b41ae9f146592e9c340ad76f33f93d88ac00000000",
  "ancestors": [
    {
      "tx": "010000000001f0550000000000001976a9144f87794c4d172c92f18589eafde7fd9a0699d8a888ac00000000"
    }
  ]
}
```

Here is a text representation:

```
TxId: a40685c30a2a1982ce9c8c115d51a7c827c7c5dd3c5abcd7df86dfcd342e857b (329 bytes)
  Version: 1
  Inputs:

    Outpoint: 0 - 5b14c9c400a840a6d4548f03d34927838b3eee478bb3de2c8d400d9d4561999f
    Script: 0x3045022100ad0884e26fbb53e50ec8f0b44c2c73ab934aa24d2088706dc9d0f364d9e9abce02204c201c6d430d3f08532bc47e62c734ec7452f2e884ebe698a545e94721db6bd441 0x029a7039e014d2dc5cb9bd10b07ab1a73773ae70e364d8691dcefa791a28ee3ccc
    Sequence: ffffffff

  Outputs:

    Value: 0.00000121
    Script: OP_DUP OP_HASH160 0x3f12d3b8fa696d3b09b61b3a8e6dffc1582f357f OP_EQUALVERIFY OP_CHECKSIG

    Value: 0.00000000
    Script: OP_0 OP_RETURN 445 OP_1 0x746573742e544b4e OP_3 0 "T1" 0x0a3c12034343591a14ffefd01ec5f7e98020e76e16b8470d13016b610d220310904e2a1a0a152095230ba1a58da05ae1f978e531d5dd147920be9110904e10a4cc9197c9c9eebe17

    Value: 0.00021862
    Script: OP_DUP OP_HASH160 0x38daf933e7b41ae9f146592e9c340ad76f33f93d OP_EQUALVERIFY OP_CHECKSIG

  LockTime: 0

Fee: 17 (0.051672 sat/byte)
Ancestors: 1
  TxId: 5b14c9c400a840a6d4548f03d34927838b3eee478bb3de2c8d400d9d4561999f (44 bytes)
  Version: 1
  Inputs:

  Outputs:

    Value: 0.00022000
    Script: OP_DUP OP_HASH160 0x4f87794c4d172c92f18589eafde7fd9a0699d8a8 OP_EQUALVERIFY OP_CHECKSIG

  LockTime: 0
  0 Miner Responses

Tokenized Test Action:
Transfer {
  "Instruments": [
    {
      "InstrumentType": "CCY",
      "InstrumentCode": "/+/QHsX36YAg524WuEcNEwFrYQ0=",
      "InstrumentSenders": [
        {
          "Quantity": 10000
        }
      ],
      "InstrumentReceivers": [
        {
          "Address": "IJUjC6GljaBa4fl45THV3RR5IL6R",
          "Quantity": 10000
        }
      ]
    }
  ],
  "OfferExpiry": 1692713873005897252
}
Instrument ID 0: CCYQLGbR1gVBKeH3axYsPSsaMZhrRh8i3xGU

```

The initiator should then verify the transaction, return a positive acknowledge to the counterparty, and post the transaction to the bitcoin network.

If there is something that the initiator doesn't approve of then they should send a negative acknowledge and both parties should drop the transaction and not broadcast it.

When the counterparty receives a positive acknowledge then they can also broadcast the transaction to the bitcoin network.

The smart contract agent will reply with a response transaction and when merkle proofs are available it will post those on the reply to channel as well.

When either party receives a response transaction from the smart contract agent or merkle proofs they should send them to the other party.


### Exchanges (Coming Soon)