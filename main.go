package main

import (
	"bytes"
	"fmt"
	"github.com/gdamore/tcell"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
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
		ui             = app.Command("ui", "Launch terminal based UI")
		importCmd      = app.Command("import", "Import transactions from raw CSV exports")
		markPayoffsCmd = app.Command("mark-payoffs", "Mark transactions that cancel each other into the 'payoffs' category")
		summary        = app.Command("summary", "Year summary")
		summary_year   = app.Flag("year", "Year").Int()
		decryptCmd     = app.Command("decrypt", "Decrypt a file")
		encryptCmd     = app.Command("encrypt", "Encrypt a file")
	)

	command := kingpin.MustParse(app.Parse(os.Args[1:]))
	log := NewLogger()
	if *verbose {
		log = log.ToWriter(os.Stdout)
	}

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

	key := []byte(os.Getenv("PENNY_SECRET_KEY"))
	pdb, err := NewPennyDb(*db, log, key)
	check(err)

	slice := pdb.Slice(startParsed, endParsed, regex, categoriesList)

	if len(slice.transactions) == 0 {
		fmt.Printf("No transactions found\n")
		return
	}

	switch command {
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
	case summary.FullCommand():
		fmt.Printf("year: %d\n", *summary_year)
		fmt.Printf("slice: %v\n", slice)
	case ui.FullCommand():
		tcell.SetEncodingFallback(tcell.EncodingFallbackASCII)
		s, e := tcell.NewScreen()

		if e != nil {
			panic(e)
		}

		if e = s.Init(); e != nil {
			panic(e)
		}

		ps := NewPennyScreen(s, slice.TableRows(false))
		ps.Display()
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
