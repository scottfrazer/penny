package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"
)

type TransactionImporter struct {
	txs map[string]*Transaction
}

func NewTransactionImporter() *TransactionImporter {
	return &TransactionImporter{make(map[string]*Transaction)}
}

func (ti *TransactionImporter) Add(tx *Transaction) {
	if _, ok := ti.txs[tx.Id()]; ok {
		for i := 0; ; i++ {
			tx.Disambiguation = fmt.Sprintf("%d", i)
			if _, ok := ti.txs[tx.Id()]; !ok {
				break
			}
		}
	}

	ti.txs[tx.Id()] = tx
}

func (ti *TransactionImporter) All() []*Transaction {
	txSlice := make([]*Transaction, len(ti.txs))
	i := 0
	for _, tx := range ti.txs {
		txSlice[i] = tx
		i++
	}

	return txSlice
}

func (importer *TransactionImporter) ImportAmazonRewards(source string, csvFileContents []byte) error {
	r := csv.NewReader(bytes.NewReader(csvFileContents))
	records, err := r.ReadAll()
	if err != nil {
		return err
	}

	for i, record := range records {
		if i == 0 {
			continue
		}

		date, err := time.Parse("01/02/2006", record[1])
		if err != nil {
			return err
		}
		amount, err := strconv.ParseFloat(record[4], 64)
		if err != nil {
			return err
		}

		importer.Add(&Transaction{source, date, record[3], amount, "", "", false})
	}
	return nil
}

func (importer *TransactionImporter) ImportDCU(source string, csvFileContents []byte) error {
	r := csv.NewReader(bytes.NewReader(csvFileContents))
	records, err := r.ReadAll()
	if err != nil {
		return err
	}

	for i, record := range records {
		if i == 0 {
			continue
		}

		memo := record[3]
		if record[2] == "SH DRAFT" {
			memo = fmt.Sprintf("Check #%s", record[7])
		} else if record[2] == "DIVIDEND" {
			memo = record[2]
		} else if record[2] == "WITHDRAW" && len(memo) == 0 {
			memo = record[2]
		}

		date, err := time.Parse("01/02/2006", record[1])
		if err != nil {
			return err
		}

		var amount float64
		if len(record[4]) > 0 {
			amount, err = strconv.ParseFloat(record[4], 64)
			if err != nil {
				return err
			}
		} else if len(record[5]) > 0 {
			amount, err = strconv.ParseFloat(record[5], 64)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("Invalid CSV")
		}

		importer.Add(&Transaction{source, date, memo, amount, "", "", false})
	}
	return nil
}

func importCapitalOne() []*Transaction {
	filename := "cap.csv"

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		fmt.Printf("Could not find file %s ... skipping\n", filename)
		return nil
	}

	fp, err := os.Open("cap.csv")
	check(err)
	defer fp.Close()

	r := csv.NewReader(fp)
	records, err := r.ReadAll()
	check(err)

	importer := NewTransactionImporter()

	for i, record := range records {
		if i == 0 {
			continue
		}

		date, err := time.Parse("01/02/2006", record[1])
		check(err)

		var amount float64
		if len(record[6]) > 0 {
			amount, err = strconv.ParseFloat(record[6], 64)
			amount = -amount
			check(err)
		} else if len(record[7]) > 0 {
			amount, err = strconv.ParseFloat(record[7], 64)
			check(err)
		} else {
			check(fmt.Errorf("Invalid CSV"))
		}

		importer.Add(&Transaction{"cap", date, record[4], amount, "", "", false})
	}

	fmt.Printf("Imported %d records from %s\n", len(records), filename)

	return importer.All()
}
