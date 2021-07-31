package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type TransactionImporter struct {
	txs         map[string]*Transaction
	investments map[string]*Investment
}

func NewTransactionImporter() *TransactionImporter {
	return &TransactionImporter{
		make(map[string]*Transaction),
		make(map[string]*Investment),
	}
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

func (ti *TransactionImporter) AddInvestment(investment *Investment) {
	if _, ok := ti.investments[investment.Id()]; ok {
		for i := 0; ; i++ {
			investment.Disambiguation = fmt.Sprintf("%d", i)
			if _, ok := ti.investments[investment.Id()]; !ok {
				break
			}
		}
	}

	ti.investments[investment.Id()] = investment
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

func (ti *TransactionImporter) AllInvestments() []*Investment {
	slice := make([]*Investment, len(ti.investments))
	i := 0
	for _, investment := range ti.investments {
		slice[i] = investment
		i++
	}

	return slice
}

func (importer *TransactionImporter) ImportInvestments(csvFileContents []byte) error {
	r := csv.NewReader(bytes.NewReader(csvFileContents))
	records, err := r.ReadAll()
	if err != nil {
		return err
	}

	cvt := func(v string) string {
		v = strings.ReplaceAll(v, "$", "")
		v = strings.ReplaceAll(v, ",", "")
		return v
	}

	normalizeDate := func(date string) string {
		r := regexp.MustCompile("(\\d+)/(\\d+)/(\\d+)")
		parts := r.FindStringSubmatch(date)
		m, _ := strconv.ParseInt(parts[1], 10, 16)
		d, _ := strconv.ParseInt(parts[2], 10, 16)
		y, _ := strconv.ParseInt(parts[3], 10, 16)
		return fmt.Sprintf("%02d/%02d/%04d", m, d, y)
	}

	for _, record := range records {
		account, err := strconv.ParseInt(record[0], 10, 64)
		if err != nil {
			continue
		}
		n := normalizeDate(record[1])
		date, err := time.Parse("01/02/2006", n)
		if err != nil {
			return err
		}
		shares, err := strconv.ParseFloat(cvt(record[4]), 64)
		if err != nil {
			return err
		}

		var price float64
		if len(record[5]) == 0 {
			price = 0.0
		} else {
			price, err = strconv.ParseFloat(cvt(record[5]), 64)
			if err != nil {
				return err
			}
		}

		importer.AddInvestment(&Investment{account, date, record[2], record[3], shares, price, ""})
	}
	return nil
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
		amount, err := strconv.ParseFloat(record[5], 64)
		if err != nil {
			return err
		}

		importer.Add(&Transaction{source, date, record[2], amount, "", "", false})
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
