package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"strings"

	dblib "github.com/facs95/decay-data/db"
	"github.com/facs95/decay-data/query"
)

func DecayLostAmounts() {
	// Set up database connection
	db, err := sql.Open("sqlite3", "./accounts.db")
	if err != nil {
		log.Fatalf("error opening database connection: %v", err)
	}
	defer db.Close()

	// Create en databases
	dblib.CreateDecayAmountTable(db)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Process the range in batches
	if err := handleProcesses(ctx, db); err != nil {
		log.Fatalf("error processing range: %v", err)
	}
}

func handleProcesses(ctx context.Context, db *sql.DB) error {
	// Create a log file to have persistent logs
	logFile, err := os.OpenFile("./decay_loss_output.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
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

	// For each account get its info
	rows, err := db.Query("select id, sender, height, amount, claim_action from claim_event order by id")
	if err != nil {
		log.Fatal("Error reading genesis addresses: ", err)
	}

	// Get all accounts from genesis on a map
	// Maybe add this to a database so I dont have to do this over and over again
	genesisClaimRecords := make(map[string]query.ClaimsRecord)
	log.Println("creating map of genesis records...")
	for _, v := range genesis.AppState.Claims.ClaimsRecords {
		genesisClaimRecords[v.Address] = v
	}
	log.Println("finished creating map of genesis records...")

	log.Println("starting to process rows...")
	decayAmounts := make(map[string]dblib.DecayAmount)
	for rows.Next() {
		var sender string
		var height string
		var id int
		var claimAction string
		var amount string
		err := rows.Scan(&id, &sender, &height, &amount, &claimAction)
		if err != nil {
			log.Printf("Error getting row: %v", err)
			continue
		}
		// create a substring by removing a word "aevmos" from string
		parsedAmount := strings.Replace(amount, "aevmos", "", 1)

		addressToChange, ok := decayAmounts[sender]
		if ok {
			doubleClaim := false
			switch claimAction {
			case "ACTION_VOTE":
				doubleClaim = addressToChange.VoteAction != ""
				addressToChange.VoteAction = parsedAmount
			case "ACTION_DELEGATE":
				doubleClaim = addressToChange.DelegateAction != ""
				addressToChange.DelegateAction = parsedAmount
			case "ACTION_EVM":
				doubleClaim = addressToChange.EVMAction != ""
				addressToChange.EVMAction = parsedAmount
			case "ACTION_IBC_TRANSFER":
				doubleClaim = addressToChange.IBCAction != ""
				addressToChange.IBCAction = parsedAmount
			}
			if doubleClaim {
				log.Printf("Double claim for address %s", sender)
			}

			//update total claimed
			amountBig := big.NewInt(0)
			amountBig, ok = amountBig.SetString(parsedAmount, 10)
			if !ok {
				log.Printf("Error converting amount to big int for address %s", sender)
				continue
			}

			claimRecord, ok := genesisClaimRecords[sender]
			if !ok {
				log.Printf("Address %s not found in genesis to calculate losses", sender)
				continue
			}

			totalClaimable, err := calculateTotalClaimable(amountBig, addressToChange.TotalClaimed)
			if err != nil {
				log.Printf("Error calculating total claimable for address %s: %v", sender, err)
			}

			totalLost, err := calculateLost(amountBig, claimRecord.InialClaimableAmount)
			if err != nil {
				log.Printf("Error calculating total lost for address %s: %v", sender, err)
			}

			addressToChange.TotalClaimed = totalClaimable
			addressToChange.TotalLost = totalLost

			totalLostEvmos, err := calculateTotalLostEvmos(totalLost)
			if err != nil {
				log.Printf("Error calculating total lost evmos for address %s: %v", sender, err)
			}
			addressToChange.TotalLostEvmos = totalLostEvmos
		} else {
			claimRecord, ok := genesisClaimRecords[sender]
			if !ok {
				log.Printf("Address %s not found in genesis to calculate losses", sender)
			}

			addressToChange = dblib.DecayAmount{
				Sender:                 sender,
				TotalClaimed:           parsedAmount,
				InitialClaimableAmount: claimRecord.InialClaimableAmount,
			}

			switch claimAction {
			case "ACTION_VOTE":
				addressToChange.VoteAction = parsedAmount
			case "ACTION_DELEGATE":
				addressToChange.DelegateAction = parsedAmount
			case "ACTION_EVM":
				addressToChange.EVMAction = parsedAmount
			case "ACTION_IBC_TRANSFER":
				addressToChange.IBCAction = parsedAmount
			}

			// Calculate the losses
			amountBig := big.NewInt(0)
			amountBig, ok = amountBig.SetString(parsedAmount, 10)
			if !ok {
				log.Printf("Error converting amount to big int for address %s", sender)
				continue
			}
			totalLost, err := calculateLost(amountBig, claimRecord.InialClaimableAmount)
			if err != nil {
				log.Printf("Error calculating total lost for address %s: %v", sender, err)
			}
			addressToChange.TotalLost = totalLost

			totalLostEvmos, err := calculateTotalLostEvmos(totalLost)
			if err != nil {
				log.Printf("Error calculating total lost evmos for address %s: %v", sender, err)
			}
			addressToChange.TotalLostEvmos = totalLostEvmos
		}

		decayAmounts[sender] = addressToChange
	}

	log.Println("Finished going through all the addresses")

	rows.Close()
	err = insertIntoDatabase(ctx, db, decayAmounts)
	if err != nil {
		log.Fatalf("Error inserting into db: %v", err)
	}

	// create a tx and submit it to the db
	return nil
}

func calculateTotalLostEvmos(totalLost string) (float64, error) {
	if totalLost == "" {
		return 0, fmt.Errorf("total lost is empty")
	}
	floatLost := big.NewFloat(0)
	floatLost, ok := floatLost.SetString(totalLost)
	if !ok {
		return 0, fmt.Errorf("Error converting amount to big float")
	}
	reducedBig := big.NewInt(0)
	reducedBig.Exp(big.NewInt(10), big.NewInt(18), nil)
	floatLost = floatLost.Quo(floatLost, big.NewFloat(0).SetInt(reducedBig))
	am, _ := floatLost.Float64()
	return am, nil
}

// calculateLost calculates the amount lost by action
func calculateLost(amountClaimed *big.Int, initialClaimableAmount string) (string, error) {
	if initialClaimableAmount == "" {
		return "", fmt.Errorf("initial claimable amount is empty")
	}
	claimableAmountBig := big.NewInt(0)
	claimableAmountBig, ok := claimableAmountBig.SetString(initialClaimableAmount, 10)
	if !ok {
		return "", fmt.Errorf("Error converting initial claimable amount to big int")
	}

	// calculate the expected amount to claim per action
	expectedAmountToClaimPerAction := claimableAmountBig.Div(claimableAmountBig, big.NewInt(4))
	// calculate the difference between the expected amount and the actual amount
	lostAmountBig := expectedAmountToClaimPerAction.Sub(expectedAmountToClaimPerAction, amountClaimed)
	return lostAmountBig.String(), nil
}

// calculateTotalClaimable calculates the total claimable amount
func calculateTotalClaimable(amountClaimed *big.Int, totalAmountClaimed string) (string, error) {
	totalClaimedBig := big.NewInt(0)
	totalClaimedBig, ok := totalClaimedBig.SetString(totalAmountClaimed, 10)
	if !ok {
		return "", fmt.Errorf("Error converting total claimed to big int for amount %v", totalAmountClaimed)
	}
	totalClaimedBig = totalClaimedBig.Add(totalClaimedBig, amountClaimed)
	return totalClaimedBig.String(), nil
}

func insertIntoDatabase(ctx context.Context, db *sql.DB, decayAmounts map[string]dblib.DecayAmount) error {
	//Create a transaction on the database
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error starting transaction: %v", err)
	}
	defer tx.Rollback()

	stmt1, err := dblib.PrepareInsertDecayAmountQuery(ctx, tx)
	if err != nil {
		return fmt.Errorf("error preparing statement for Table1: %v", err)
	}
	defer stmt1.Close()

	for _, d := range decayAmounts {
		err := dblib.ExecContextDecayAmount(ctx, stmt1, d)
		if err != nil {
			return fmt.Errorf("error inserting data into Table1: %v", err)
		}
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("error committing transaction: %v", err)
	}

	return nil
}
