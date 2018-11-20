package main

import (
	"fmt"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func main() {
	log := NewLogger().ToWriter(os.Stdout)

	defaultStart := time.Now().Add(-365 * 24 * time.Hour).Format("01/02/2006")
	defaultEnd := time.Now().Format("01/02/2006")

	var (
		app                  = kingpin.New("finance", "A command-line finance manager")
		db                   = app.Flag("db", "Path to database file").Default("penny.sqlite3.encrypted").String()
		start                = app.Flag("start", "Start date (MM/DD/YYYY)").Default(defaultStart).String()
		end                  = app.Flag("end", "End date (MM/DD/YYYY)").Default(defaultEnd).String()
		categories           = app.Flag("category", "Filter by categories").String()
		regexString          = app.Flag("regex", "Filter by regular expression").String()
		list                 = app.Command("list", "List transactions")
		edit                 = app.Command("edit", "Edit transactions")
		importCmd            = app.Command("import", "Import transactions from raw CSV exports")
		decryptCmd           = app.Command("decrypt", "Decrypt the SQLite3 database")
		decryptCmdOutputPath = decryptCmd.Arg("outfile", "Where to write the decrypted database").Default("penny.sqlite3").String()
	)

	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	startParsed, err := time.Parse("01/02/2006", *start)
	if err != nil {
		fmt.Printf("Invalid start date %s: %s\n", *start, err)
		os.Exit(1)
	}

	endParsed, err := time.Parse("01/02/2006", *end)
	if err != nil {
		fmt.Printf("Invalid end date %s: %s\n", *end, err)
		os.Exit(1)
	}

	regex := regexp.MustCompile(*regexString)
	categoriesList := []string{}
	if len(*categories) > 0 {
		categoriesList = strings.Split(*categories, ",")
	}

	pdb, err := NewPennyDb(*db, log, []byte(os.Getenv("PENNY_SECRET_KEY")))
	check(err)

	slice := pdb.Slice(startParsed, endParsed, regex, categoriesList)

	if len(slice.transactions) == 0 {
		fmt.Printf("No transactions found\n")
		return
	}

	switch command {
	case decryptCmd.FullCommand():
		pdb.WriteDecryptedDb(*decryptCmdOutputPath)
	case importCmd.FullCommand():
		importer := NewTransactionImporter()

		chaseCsvContents, err := ioutil.ReadFile("chase.csv")
		if err != nil {
			fmt.Printf("Could not read file chase.csv... skipping (%v)\n", err)
		} else {
			importer.ImportAmazonRewards("chase", chaseCsvContents)
		}

		dcuCsvContents, err := ioutil.ReadFile("dcu.csv")
		if err != nil {
			fmt.Printf("Could not read file dcu.csv... skipping (%v)\n", err)
		} else {
			importer.ImportDCU("dcu", dcuCsvContents)
		}

		dcu2CsvContents, err := ioutil.ReadFile("dcu2.csv")
		if err != nil {
			fmt.Printf("Could not read file dcu2.csv... skipping (%v)\n", err)
		} else {
			importer.ImportDCU("dcu2", dcu2CsvContents)
		}

		dcu3CsvContents, err := ioutil.ReadFile("dcu3.csv")
		if err != nil {
			fmt.Printf("Could not read file dcu3.csv... skipping (%v)\n", err)
		} else {
			importer.ImportDCU("dcu3", dcu3CsvContents)
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
		pdb.SaveEditTsv(contents)
	}
}
