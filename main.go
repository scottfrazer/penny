package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/olekukonko/tablewriter"
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
		report         = app.Command("report", "Generate Report")
		investments    = app.Command("investments", "Investments")
		sqlite         = app.Command("sqlite", "Get SQLite shell for database. CTRL-D to exit and save")
	)

	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	key := []byte(os.Getenv("PENNY_SECRET_KEY"))

	switch command {
	case encryptCmd.FullCommand():
		contents, err := ioutil.ReadAll(os.Stdin)
		check(err)
		ciphertext, err := encrypt(key, contents)
		check(err)
		os.Stdout.Write(ciphertext)
		os.Exit(0)
	case decryptCmd.FullCommand():
		contents, err := ioutil.ReadAll(os.Stdin)
		check(err)
		plaintext, err := decrypt(key, contents)
		check(err)
		os.Stdout.Write(plaintext)
		os.Exit(0)
	}

	log := NewLogger()
	if *verbose {
		log = log.ToWriter(os.Stdout)
	}

	pdb, err := NewPennyDb(*db, log, key)
	check(err)

	switch command {
	case sqlite.FullCommand():
		handle, err := pdb.OpenReadWrite()
		check(err)

		cmd := exec.Command("sqlite3", handle.decryptedDbPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Run(); err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode := exitError.Sys().(syscall.WaitStatus).ExitStatus()
				log.Info("exit code %d for sqlite3 process", exitCode)
			}
		} else {
			exitCode := cmd.ProcessState.Sys().(syscall.WaitStatus)
			log.Info("exit code %d for sqlite3 process", exitCode)
		}

		handle.Close()
		os.Exit(0)
	case report.FullCommand():
		txs := pdb.AllTransactions()
		firstMonth := txs[0].Date.Month()
		firstYear := txs[0].Date.Year()
		firstQuarter := monthToQuarter(int(firstMonth))

		quarter := firstQuarter
		year := firstYear

		type Quarter struct {
			quarter int
			year    int
			slice   *TxSlice
		}

		var quarters []Quarter

		for {
			start, end, err := quarterToDateRange(int(quarter), int(year))
			check(err)

			slice := pdb.Slice(&Filter{nil, nil, start, end})

			if len(slice.transactions) == 0 {
				break
			}

			quarters = append(quarters, Quarter{quarter, year, slice})

			quarter++
			if quarter == 5 {
				quarter = 1
				year++
			}
		}

		writer := os.Stdout
		table := tablewriter.NewWriter(writer)
		table.SetHeader([]string{
			"Quarter",
			"Income",
			"Expenses",
			"Investments",
			"Monthly Expenses",
			"Savings Rate",
		})

		var savingsRateSum float64
		var incomeSum float64
		var expensesSum float64
		var investmentSum float64
		var expensesMonthlySum float64

		for _, q := range quarters {
			var income float64
			var expenses float64
			var investment float64

			for _, tx := range q.slice.transactions {
				if tx.Ignored && strings.Contains(tx.Memo, "VANGUARD BUY INVESTMENT") {
					investment += -tx.Amount
					continue
				}

				if tx.Category == "payoff" || tx.Ignored == true {
					continue
				}

				if tx.Category == "income" {
					income += tx.Amount
				} else {
					expenses += tx.Amount
				}
			}

			expensesMonthly := (expenses / q.slice.ElapsedDays()) * 30.0
			rate := (1 - (-(expenses / income))) * 100

			incomeSum += income
			expensesSum += expenses
			investmentSum += investment
			expensesMonthlySum += expensesMonthly
			savingsRateSum += rate

			table.Append([]string{
				fmt.Sprintf("Q%d %d", q.quarter, q.year),
				money(income, true),
				money(expenses, true),
				money(investment, true),
				money(expensesMonthly, true),
				fmt.Sprintf("%.1f%%", rate),
			})
			if q.quarter == 4 {
				table.Append([]string{"", "", "", "", "", ""})
			}
		}

		size := float64(len(quarters))
		table.SetFooter([]string{
			"AVERAGE",
			money(incomeSum/size, false),
			money(expensesSum/size, false),
			money(investmentSum/size, false),
			money(expensesMonthlySum/size, false),
			fmt.Sprintf("%.1f%%", savingsRateSum/float64(len(quarters))),
		})
		table.Render()
		io.WriteString(writer, "\n")
		os.Exit(0)
	}

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
	case investments.FullCommand():
		investments := pdb.AllInvestments()
		cache, err := NewStockSymbolLookup(pdb)
		check(err)

		writer := os.Stdout
		table := tablewriter.NewWriter(writer)
		table.SetHeader([]string{
			"Account",
			"Date",
			"Type",
			"Symbol",
			"Shares",
			"Price",
			"Purchase",
			"Value",
			"Profit",
		})

		for _, investment := range investments {
			price, err := cache.Get(investment.Symbol)
			check(err)
			value := investment.Shares * price
			purchase := investment.Shares * investment.Price
			profit := value - purchase
			table.Append([]string{
				fmt.Sprintf("%d", investment.Account),
				investment.Date.Format("01/02/2006"),
				investment.Type,
				investment.Symbol,
				fmt.Sprintf("%.2f", investment.Shares),
				money(investment.Price, true),
				money(purchase, true),
				money(value, true),
				money(profit, true),
			})
		}
		table.Render()

		grouped := make(map[string][]*Investment)
		for _, investment := range investments {
			group := fmt.Sprintf("%d-%s", investment.Account, investment.Symbol)
			grouped[group] = append(grouped[group], investment)
		}

		table = tablewriter.NewWriter(writer)
		table.SetHeader([]string{
			"Account",
			"Symbol",
			"Shares",
			"Purchase",
			"Value",
			"Profit",
		})
		for _, investments := range grouped {
			var shares, purchase, value float64
			account := investments[0].Account
			symbol := investments[0].Symbol
			price, err := cache.Get(symbol)
			check(err)
			for _, investment := range investments {
				shares += investment.Shares
				purchase += investment.Shares * investment.Price
				value += investment.Shares * price
			}
			table.Append([]string{
				fmt.Sprintf("%d", account),
				symbol,
				fmt.Sprintf("%.2f", shares),
				money(purchase, true),
				money(value, true),
				money(value-purchase, true),
			})
		}
		table.Render()
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

		investmentContents, err := ioutil.ReadFile("investments.csv")
		if err != nil {
			fmt.Printf("Could not read file investments.csv... skipping (%v)\n", err)
		} else {
			check(importer.ImportInvestments(investmentContents))
		}

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
		x := importer.AllInvestments()
		for _, y := range x {
			fmt.Printf("%v\n", y)
		}
		err = pdb.InsertInvestments(x)
		if err != nil {
			fmt.Println(err)
		}
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
