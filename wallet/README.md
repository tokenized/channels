# Wallet

Wallet provides a simple system for managing keys and bitcoin.

# Keys

A single base key is specified, then new keys are derived from that by calculating hashes and adding them to the key value. These derived keys can by used to receive payments or as base keys in Channels relationships.

# Context IDs

Context IDs are used to identify specific interactions. So when a payment is sent or a request is received it is given a randomized context ID that is used to later identify the keys.
