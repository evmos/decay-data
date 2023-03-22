This repo tries to efficiently collect migrated and merged events over a specified block range.

## Results

The collected data will be generated under an `account.db` sqlite3 database containing the following tables:

```go
type MergedEvent struct {
	ID                       int
	Recipient                string
	Sender                   string
	ClaimedCoins             string
	FundCommunityPool        string
	SenderEvmosPrefix        string
	SenderGenesisClaimRecord string
	Height                   int
}

type ClaimEvent struct {
	ID     int
	Sender string
	Action string
	Amount string
	Height int
}

type DecayAmount struct {
	ID                     int
	Sender                 string
	VoteAction             string
	DelegateAction         string
	EVMAction              string
	IBCAction              string
	TotalClaimed           string
	TotalLost              string
	InitialClaimableAmount string
	TotalLostEvmos         float64
}

type Error struct {
	ID         int
	Height     int
	EventType  string
	TxIndex    string
	EventIndex string
}
```

## Instructions

1. Run `go build` to install the binary
2. Run `./decay-data collect-events FROM_BLOCK TO_BLOCK`  with the block range you want to collect the events from
    - Note that this will take some time depending on the number of blocks that wants to be inspected
3. Run `./decay-data collect-merge-senders`
4. Run `./decay-data calculate-decay-loss`
5. Run `./decay-data sender-evmos-prefix`
6. New database `accounts.db` should be generated in the root directory.

## How it works

- `collect-events`
    - Collects `claim` and `merge_claims_records` into a sqlite3 database through the following steps on every block within the specified range:
        - Creates `accounts.db` sqlite3 database
        - Queries `BlockResults`
        - Iterates over all of the Txs within the block
        - If `Tx` emitted either `merge_claims_records` then it decodes the attributes of the event and stores the event in `merged_event` table
        - If `Tx` emitted either `claim` then it decodes the attributes of the event and stored the event in `claim_event` table
    - Required Params
        - `fromBlock`
            - Block height to start collecting events from
        - `toBlock`
            - End height of block range
    - Results
        - Generates a sqlite3 database, `accounts.db`
        - There are three tables within the database with the following models

            ```go
            type MergedEvent struct {
            	ID                 int
            	Recipient          string
            	Sender             string
            	ClaimedCoins       string
            	FundCommunityPool  string
            	SenderEvmosPrefix  string
            	SenderGenesisClaimRecord string
            	Height             int
            }

            type ClaimEvent struct {
            	ID     int
            	Sender string
            	Action string
            	Amount string
            	Height int
            }

            type Error struct {
            	ID         int
            	Height     int
            	EventType  string
            	TxIndex    string
            	EventIndex string
            }
            ```

- `collect-merge-senders`
    - Because the `sender` of the transaction is not present on the `merge_claims_records` event, an extra step was necessary to collect the sender and track the claims properly. To accomplish this the following steps were followed:
        - Iterate over all the events on the `merged_event` table in `accounts.db`
        - For every event, query `BlockResult`
        - Find `Tx` within block
        - Find `recv_packet` event within `Tx`
        - Decode attributes of the event and add the `sender` to the row record within `MergeEvent` table
- `calculate-decay-loss`
    - Using Evmos’s genesis file claims records and the acquired data on `claim_event`, calculate per address:
        - `TotalClaimed` - Total amount of `aevmos` claimed
        - `InitialClaimableAmount` - Claimable amount at genesis
        - `TotalLost` - Total amount lost in `aevmos`
        - `TotalLostEvmos` - Total amount lost in `evmos`
        - `EVMAction` - Amount claimed through `EVMAction` claim type
        - `IBCAction` - Amount claimed through `IBCAction` claim type
        - `VoteAction` - Amount claimed through `VoteAction` claim type
        - `DelegateAction` - Amount claimed through `DelegateAction` claim type
    - Results
        - A New `DecayAmount` table gets generated with the following model

            ```go
            type DecayAmount struct {
            	ID                     int
            	Sender                 string
            	VoteAction             string
            	DelegateAction         string
            	EVMAction              string
            	IBCAction              string
            	TotalClaimed           string
            	TotalLost              string
            	InitialClaimableAmount string
            	TotalLostEvmos         float64
            }
            ```

- `sender-evmos-prefix`
    - This script will basically populate the `SenderEvmosPrefix` column in `MergedEvent` table. This is because the sender on each `merge_claims_records` event is on `osmosis` denomination, we need to find the equivalent `evmos` address to be able to find its respective `initial_claimable_record` in genesis.
