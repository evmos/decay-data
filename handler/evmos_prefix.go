package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"os"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	dblib "github.com/facs95/decay-data/db"
	"github.com/facs95/decay-data/query"
	_ "github.com/mattn/go-sqlite3"
)

const (
	EvmosPrefix   = "evmos"
	OsmosisPrefix = "osmo"
)

func GetEvmosPrefix() {
	// Create a log file to have persistent logs
	logFile, err := os.OpenFile("./output.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer logFile.Close()

	wrt := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(wrt)

	// Collect claim records from genesis
	content, err := os.ReadFile("genesis.json")
	if err != nil {
		log.Fatalf("Error reading the genesis: %q", err)
	}

	var genesis query.Genesis
	err = json.Unmarshal(content, &genesis)
	if err != nil {
		log.Fatal("Error unmarshalling genesis: ", err)
	}

	// Get all accounts from genesis on a map
	// Maybe add this to a database so I dont have to do this over and over again
	genesisClaimRecords := make(map[string]query.ClaimsRecord)
	log.Println("creating map of genesis records...")
	for _, v := range genesis.AppState.Claims.ClaimsRecords {
		genesisClaimRecords[v.Address] = v
	}
	log.Println("finished creating map of genesis records...")

	// Set up database connection
	db, err := sql.Open("sqlite3", "./accounts.db")
	if err != nil {
		log.Fatalf("error opening database connection: %v", err)
	}
	defer db.Close()

	// For each account get its info
	rows, err := db.Query("select id, sender from merged_event order by id")
	if err != nil {
		log.Fatalf("Error reading addresses %v", err)
	}
	defer rows.Close()

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var batchOfEventsToUpdate []dblib.MergedEvent

	for rows.Next() {
		var sender string
		var id int
		err := rows.Scan(&id, &sender)
		if err != nil {
			log.Fatalf("Error getting row: %v", err)
		}

		senderEvmosPrefix, err := ConvertAddress(sender, OsmosisPrefix, EvmosPrefix)
		if err != nil {
			log.Printf("Error converting address %s: %v", sender, err)
			continue
		}

		claimRecord, ok := genesisClaimRecords[senderEvmosPrefix]
		if !ok {
			log.Printf("Address %s not found in genesis to calculate losses", senderEvmosPrefix)
			continue
		}

		eventToUpdate := dblib.MergedEvent{
			ID:                       id,
			SenderEvmosPrefix:        senderEvmosPrefix,
			SenderGenesisClaimRecord: claimRecord.InialClaimableAmount,
		}

		batchOfEventsToUpdate = append(batchOfEventsToUpdate, eventToUpdate)

	}

	log.Println("Rows scan finished")
	rows.Close()

	for _, eventToUpdate := range batchOfEventsToUpdate {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			log.Printf("error starting transaction: %v", err)
			return
		}
		defer tx.Rollback()

		stmt, err := dblib.PrepareUpdateEvmosSenderPrefix(ctx, tx)
		if err != nil {
			log.Printf("error preparing statement for update: %v", err)
			return
		}
		defer stmt.Close()

		//update events in database by ID
		if err := dblib.ExecContextMergeEventEvmosPrefixUpdate(ctx, stmt, eventToUpdate); err != nil {
			log.Printf("error updating merged event: %v", err)
			continue
		}

		// Commit the transaction
		err = tx.Commit()
		if err != nil {
			log.Printf("Are we here?")
			log.Printf("error committing transaction: %v", err)
			return
		}
	}

	db.Close()
	log.Println("Job finished")
}

func ConvertAddress(address, originalPrefix, newPrefix string) (string, error) {
	bz, err := sdk.GetFromBech32(address, originalPrefix)
	if err != nil {
		return "", err
	}
	return bech32.ConvertAndEncode(newPrefix, bz)
}
