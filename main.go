package main

import (
	"os"
	"strconv"

	"github.com/facs95/decay-data/handler"
)

func main() {
	if len(os.Args) == 1 {
		panic("No arguments provided. Please provide either 'collect-events' or 'collect-merge-senders'")
	}

	if os.Args[1] == "collect-events" {
		if len(os.Args) != 4 {
			panic("Not enough arguments provided. Please provide block range to query")
		}

		fromBlock, err := strconv.Atoi(os.Args[2])
		if err != nil {
			panic("fromBlock is not a number")
		}

		toBlock, err := strconv.Atoi(os.Args[3])
		if err != nil {
			panic("toBlock is not a number")
		}

		handler.CollectEvents(fromBlock, toBlock)
	} else if os.Args[1] == "collect-merge-senders" {
		handler.CollectMergeSenders()
	} else if os.Args[1] == "calculate-decay-loss" {
		handler.DecayLostAmounts()
	} else if os.Args[1] == "sender-evmos-prefix" {
		handler.GetEvmosPrefix()
	} else {
		panic("Invalid argument provided. Please provide either 'collect-events' or 'collect-merge-senders'")
	}
}
