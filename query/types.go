package query

import "encoding/base64"

type BlockResult struct {
	Result Result `json:"result"`
	Height int64  `json:"height"`
}

type Result struct {
	TxsResults []ResponseDeliverTx `json:"txs_results"`
}

type ResponseDeliverTx struct {
	Events []Event `json:"events,omitempty"`
}

type Event struct {
	Type       string      `json:"type"`
	Attributes []Attribute `json:"attributes"`
}

func (be *Event) DecodeAttributes() error {
	for i := range be.Attributes {
		decoded, err := base64.StdEncoding.DecodeString(be.Attributes[i].Value)
		if err != nil {
			return err
		}
		be.Attributes[i].Value = string(decoded)
	}
	return nil
}

type Attribute struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
	Index bool   `json:"index,omitempty"`
}

type PacketData struct {
	Amount   string `json:"amount"`
	Denom    string `json:"denom"`
	Receiver string `json:"receiver"`
	Sender   string `json:"sender"`
}

// Genesis data
type Genesis struct {
	AppState AppState `json:"app_state"`
}

type AppState struct {
	Claims Claims `json:"claims"`
}

type Claims struct {
	ClaimsRecords []ClaimsRecord `json:"claims_records"`
}

type ClaimsRecord struct {
	Address              string `json:"address"`
	ActionsCompleted     []bool `json:"actions_completed"`
	InialClaimableAmount string `json:"initial_claimable_amount"`
}
