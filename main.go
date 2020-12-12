package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	defaultStart := time.Now().Add(-365 * 24 * time.Hour).Format("01/02/2006")
	defaultEnd := time.Now().Format("01/02/2006")

	var (
		app            = kingpin.New("finance", "A command-line finance manager")
		verbose        = app.Flag("verbose", "Verbose output").Short('v').Bool()
		db             = app.Flag("db", "Path to database file").Default("penny.sqlite3.encrypted").String()
		start          = app.Flag("start", "Start date (MM/DD/YYYY)").Default(defaultStart).String()
		end            = app.Flag("end", "End date (MM/DD/YYYY)").Default(defaultEnd).String()
		categories     = app.Flag("category", "Filter by categories").String()
		regexString    = app.Flag("regex", "Filter by regular expression").String()
		list           = app.Command("list", "List transactions")
		edit           = app.Command("edit", "Edit transactions")
		importCmd      = app.Command("import", "Import transactions from raw CSV exports")
		markPayoffsCmd = app.Command("mark-payoffs", "Mark transactions that cancel each other into the 'payoffs' category")
		decryptCmd     = app.Command("decrypt", "Decrypt a file")
		encryptCmd     = app.Command("encrypt", "Encrypt a file")
		server         = app.Command("server", "Start Server")
	)

	command := kingpin.MustParse(app.Parse(os.Args[1:]))
	log := NewLogger()
	if *verbose {
		log = log.ToWriter(os.Stdout)
	}

	key := []byte(os.Getenv("PENNY_SECRET_KEY"))
	pdb, err := NewPennyDb(*db, log, key)
	check(err)

	filter, errors := ParseFilter(RawFilter{*categories, *regexString, *start, *end})
	if len(errors) != 0 {
		for k, v := range errors {
			fmt.Fprintf(os.Stderr, "ERROR: %s: %s", k, v)
		}
		os.Exit(1)
	}

	slice := pdb.Slice(filter)

	if len(slice.transactions) == 0 {
		fmt.Printf("No transactions found\n")
		return
	}

	switch command {
	case server.FullCommand():
		PennyHTTPServer(":8090", pdb)
	case markPayoffsCmd.FullCommand():
		slice.MarkPayoffs(log)
		tsv := slice.GetEditTsv()
		// reload the slice
		pdb2, err := NewPennyDb(*db, log, key)
		check(err)
		// apply the edits
		check(pdb2.SaveEditTsv(bytes.NewReader(tsv)))
	case encryptCmd.FullCommand():
		contents, err := ioutil.ReadAll(os.Stdin)
		check(err)
		ciphertext, err := encrypt(key, contents)
		check(err)
		os.Stdout.Write(ciphertext)
	case decryptCmd.FullCommand():
		contents, err := ioutil.ReadAll(os.Stdin)
		check(err)
		plaintext, err := decrypt(key, contents)
		check(err)
		os.Stdout.Write(plaintext)
	case importCmd.FullCommand():
		importer := NewTransactionImporter()

		chaseCsvContents, err := ioutil.ReadFile("chase.csv")
		if err != nil {
			fmt.Printf("Could not read file chase.csv... skipping (%v)\n", err)
		} else {
			check(importer.ImportAmazonRewards("chase", chaseCsvContents))
		}

		dcuCsvContents, err := ioutil.ReadFile("dcu.csv")
		if err != nil {
			fmt.Printf("Could not read file dcu.csv... skipping (%v)\n", err)
		} else {
			check(importer.ImportDCU("dcu", dcuCsvContents))
		}

		dcu2CsvContents, err := ioutil.ReadFile("dcu2.csv")
		if err != nil {
			fmt.Printf("Could not read file dcu2.csv... skipping (%v)\n", err)
		} else {
			check(importer.ImportDCU("dcu2", dcu2CsvContents))
		}

		dcu3CsvContents, err := ioutil.ReadFile("dcu3.csv")
		if err != nil {
			fmt.Printf("Could not read file dcu3.csv... skipping (%v)\n", err)
		} else {
			check(importer.ImportDCU("dcu3", dcu3CsvContents))
		}

		pdb.Insert(importer.All())
	case list.FullCommand():
		slice.WriteHumanReadableTable(os.Stdout)
		fmt.Printf("\n\n")
		slice.WriteHumanReadableTotals(os.Stdout)
	case edit.FullCommand():
		tmpfile, err := ioutil.TempFile("", "")
		check(err)
		defer os.Remove(tmpfile.Name())

		_, err = tmpfile.Write(slice.GetEditTsv())
		check(err)

		err = tmpfile.Close()
		check(err)

		cmd := exec.Command("vim", tmpfile.Name())
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		err = cmd.Run()
		check(err)

		contents, err := ioutil.ReadFile(tmpfile.Name())
		check(err)
		pdb.SaveEditTsv(bytes.NewReader(contents))
	}
}
