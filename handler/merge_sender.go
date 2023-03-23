package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"os"
	"strconv"
	"sync"

	dblib "github.com/facs95/decay-data/db"
	"github.com/facs95/decay-data/query"
	_ "github.com/mattn/go-sqlite3"
)

func CollectMergeSenders() {
	// Create a log file to have persistent logs
	logFile, err := os.OpenFile("./output.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer logFile.Close()

	wrt := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(wrt)

	var accountsToProcess []dblib.MergedEvent
	// Set up database connection
	db, err := sql.Open("sqlite3", "./accounts.db")
	if err != nil {
		log.Fatalf("error opening database connection: %v", err)
	}
	defer db.Close()

	// For each account get its info
	rows, err := db.Query("select id, recipient, height, claimed_coins, fund_community_pool_coins from merged_event order by id")
	if err != nil {
		log.Fatalf("Error reading addresses %v", err)
	}
    defer rows.Close()


	for rows.Next() {
		var address string
		var height int
		var id int
		var claimedCoins string
		var fundCommunityPoolCoins string
		err := rows.Scan(&id, &address, &height, &claimedCoins, &fundCommunityPoolCoins)
		if err != nil {
			log.Fatalf("Error getting row: %v", err)
		}
		accountsToProcess = append(accountsToProcess, dblib.MergedEvent{Recipient: address, Height: height, ID: id, ClaimedCoins: claimedCoins, FundCommunityPool: fundCommunityPoolCoins})
	}

	log.Println("Finished getting all the addresses")
	rows.Close()

	if err := orchestrator(db, accountsToProcess); err != nil {
		log.Printf("Error executing the orchestrator: %v", err)
	}

	db.Close()
	log.Println("Job finished")
}

// Add a column to the same table with the sender

// Do the same go ruotine process and query each block

// Once I have the block the only thing I need is to look for the `recv_packet` event
// get the first attribute
// And from here get the sender

func orchestrator(db *sql.DB, items []dblib.MergedEvent) error {
	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a channel to hold jobs to be executed by workers
	jobs := make(chan []dblib.MergedEvent, MaxWorkers)
	// Create a WaitGroup to wait for all workers to complete
	wg := sync.WaitGroup{}

	// Launch worker goroutines
	for i := 0; i < MaxWorkers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for job := range jobs {
				log.Printf("starting worker %v with events %v-%v", i, job[0].ID, job[len(job)-1].ID)
				// Query the external resource for data
				queueOfEventsToUpdate := processBatchOfEvents(job)

				// Process the data and insert into MySQL database
				updateQueueOfEventsToUpdate(db, ctx, queueOfEventsToUpdate)
				log.Printf("finished worker %v", i)
			}
		}(i)
	}

	limit := len(items)
	log.Printf("total events to process: %v", limit)
	// Generate jobs for each batch and send them to the jobs channel
	for i := 0; i < limit; i += BatchSize {
		end := i + BatchSize

		if end > limit {
			end = limit
		}

		// produce a copy to avoid concurrent issues
		jobs <- copySliceOfStructs(items[i:end])
	}

	close(jobs)
	wg.Wait()
	return nil
}

func updateQueueOfEventsToUpdate(db *sql.DB, ctx context.Context, queueOfEventsToUpdate []dblib.MergedEvent) {
	//Create a transaction on the database
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("error starting transaction: %v", err)
		return
	}
	defer tx.Rollback()

	stmt, err := dblib.PrepareUpdateSenderMergeEventQuery(ctx, tx)
	if err != nil {
		log.Printf("error preparing statement for update: %v", err)
		return
	}
	defer stmt.Close()

	//update events in database by ID
	for _, event := range queueOfEventsToUpdate {
		if err := dblib.ExecContextMergeEventUpdate(ctx, stmt, event); err != nil {
			log.Printf("error updating merged event: %v", err)
			continue
		}
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		log.Printf("error committing transaction: %v", err)
		return
	}
}

// copy slice of structs
func copySliceOfStructs(s []dblib.MergedEvent) []dblib.MergedEvent {
	c := make([]dblib.MergedEvent, len(s))
	copy(c, s)
	return c
}

func processBatchOfEvents(events []dblib.MergedEvent) []dblib.MergedEvent {
	queueOfEventsToUpdate := []dblib.MergedEvent{}
	for _, event := range events {
		blockResult, err := query.GetBlockResult(strconv.Itoa(event.Height), 0)
		if err != nil {
			log.Printf("error getting block result: %v", err)
			continue
		}
		tx, found := findTxWithinBlockResultTxs(event, blockResult.Result.TxsResults)
		if !found {
			log.Printf("error finding tx within block result txs for event: %v", event.ID)
			continue
		}
		sender, found := findSenderWithinEvents(tx)
		if !found {
			log.Printf("error finding sender for event: %v", event.ID)
			continue
		}
		queueOfEventsToUpdate = append(queueOfEventsToUpdate, dblib.MergedEvent{ID: event.ID, Sender: sender})
	}
	return queueOfEventsToUpdate
}

// findTxWithinBlockResultTxs finds the tx within the block result txs
// - Iterates over all of the txs in the block
// - It looks for the event type `merge_claims_records`
// - It decodes the attributes
// - It checks if the attributes match the event
func findTxWithinBlockResultTxs(event dblib.MergedEvent, txs []query.ResponseDeliverTx) (tx query.ResponseDeliverTx, found bool) {
	//  Iterate over all txs in the block
	for i := range txs {
		// Iterate over all events in tx
		for index := range txs[i].Events {
			switch t := txs[i].Events[index].Type; t {
			case "merge_claims_records":
				v := txs[i].Events[index]
				// Decode the attributes
				err := v.DecodeAttributes()
				if err != nil {
					log.Printf("error decoding resource at height %v", err)
					// we should add this records to error table
					// return nil, nil
					continue
				}
				isTx := isTransaction(event, v.Attributes)
				if isTx {
					return txs[i], true
				}
			}
		}
	}
	return query.ResponseDeliverTx{}, false
}

// findSenderWithinEvents find sender on recv_packet event
// - Looks for recv_packet event within the tx
// - Decodes the packet data and gets the tx sender from it
// NOTE: `merge_claims_records` is always triggered by an IBC transaction, hence we can
// guarantee that the `recv_packet` event will be present
func findSenderWithinEvents(tx query.ResponseDeliverTx) (sender string, found bool) {
	// Iterate over all events in tx
	for eventIndex := range tx.Events {
		switch t := tx.Events[eventIndex].Type; t {
		case "recv_packet":
			v := tx.Events[eventIndex]
			// Decode the attributes
			err := v.DecodeAttributes()
			if err != nil {
				log.Printf("error decoding resource at height %v", err)
				// we should add this records to error table
				// return nil, nil
				continue
			}

			// unmarshal the packet data
			packetData := &query.PacketData{}
			err = json.Unmarshal([]byte(v.Attributes[0].Value), &packetData)
			if err != nil {
				log.Printf("error unmarshalling packet data: %v", err)
				continue
			}

			return packetData.Sender, true

		}
	}
	return "", false
}

// find event within array of dblib.mergedEvents based on the Recipient and height
func isTransaction(event dblib.MergedEvent, attributes []query.Attribute) bool {
	if event.Recipient == attributes[0].Value &&
		event.ClaimedCoins == attributes[1].Value &&
		event.FundCommunityPool == attributes[2].Value {
		return true
	}
	return false
}
