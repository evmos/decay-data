package db

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
