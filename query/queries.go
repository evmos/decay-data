package query

import (
	"encoding/json"
	"net/http"
	"time"

	"io/ioutil"
	"log"
)

var client = &http.Client{}
var clientUrl = "https://tendermint.bd.evmos.org:26657/"

// GetBlockResult queries `block_result` directly from node
// if the request or parser fail the function  will retry 3 times
func GetBlockResult(height string, try int) (*BlockResult, error) {
	try += try
	balance_start := "block_results?height="
	url := balance_start + height
	body, err := makeRequest(url, height)
	if err != nil {
		if try >= 3 {
			return nil, err
		}
		time.Sleep(1000)
		return GetBlockResult(height, try)
	}
	m := &BlockResult{}
	err = json.Unmarshal(body, &m)
	if err != nil {
		if try >= 3 {
			return nil, err
		}
		time.Sleep(1000)
		return GetBlockResult(height, try)
	}
	return m, nil
}

func makeRequest(endpoint string, height string) ([]byte, error) {
	req, _ := http.NewRequest("GET", clientUrl+endpoint, nil)
	res, err := client.Do(req)
	if err != nil {
		log.Println("Error http request", endpoint, err)
		return nil, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Println("Error reading the response body", endpoint, err)
		return nil, err
	}
	return body, nil
}
