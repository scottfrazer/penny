package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/olekukonko/tablewriter"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	defaultStart := time.Now().Add(-365 * 24 * time.Hour).Format("01/02/2006")
	defaultEnd := time.Now().Format("01/02/2006")

	var (
		app            = kingpin.New("penny", "A command-line day manager")
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
		report         = app.Command("report", "Generate Report")
		investments    = app.Command("investments", "Show investment table")
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
		year := txs[0].Date.Year()
		quarter := monthToQuarter(int(txs[0].Date.Month()))

		var quarters Quarters

		for {
			start, end, err := quarterToDateRange(int(quarter), int(year))
			check(err)

			slice := pdb.Slice(&Filter{nil, nil, start, end})

			if len(slice.transactions) == 0 {
				break
			}

			quarters = append(quarters, Quarter{quarter, year, slice})

			if quarter == 4 {
				quarter = 1
				year++
			} else {
				quarter++
			}
		}

		avgExpenses := 0.0
		for n := 0; n < 8; n++ {
			avgExpenses += quarters[len(quarters)-n-1].Expenses()
		}
		avgExpenses = avgExpenses / 8

		stockLookup, err := NewStockSymbolLookup(pdb)
		check(err)
		investmentTotal := 0.0
		for _, investment := range pdb.AllInvestments() {
			currentPrice, err := stockLookup.Get(investment.Symbol)
			check(err)
			investmentTotal += investment.Shares * currentPrice
		}

		writer := os.Stdout
		table := tablewriter.NewWriter(writer)
		table.SetHeader([]string{
			"Quarter",
			"Income",
			"Expenses",
			"Investments",
			"Savings Rate",
		})

		for _, q := range quarters {
			table.Append([]string{
				fmt.Sprintf("Q%d %d", q.quarter, q.year),
				money(q.Income(), true),
				money(q.Expenses(), true),
				money(q.Investments(), true),
				fmt.Sprintf("%.1f%%", q.SavingsRate()),
			})
			if q.quarter == 4 {
				table.Append([]string{"", "", "", "", ""})
			}
		}

		table.SetFooter([]string{
			"AVERAGE",
			money(quarters.AvgIncome(), false),
			money(quarters.AvgExpenses(), false),
			money(quarters.AvgInvestments(), false),
			fmt.Sprintf("%.1f%%", quarters.AvgSavingsRate()),
		})
		table.Render()
		io.WriteString(writer, "\n")

		life_expectancy := 90
		ror := .07 // nominal
		annual_contribution := 40000.0
		ror_retirement := .05 // nominal
		inflation := .03
		expense_growth := func(year int) float64 {
			return FV(inflation, float64(year-2021), 0, 100000.0, false)
		}
		// retirement_expense_function := func(year int) float64 {
		// 	return 0.0
		// }

		investmentTotal += 124000.0
		firecalc, err := pdb.DBBackedCache("firecalc_cache", func(s string) string {
			var portfolio, expenses float64
			var years int
			_, err := fmt.Sscanf(s, "%f,%f,%d", &portfolio, &expenses, &years)
			check(err)
			successRate, _ := FIRECalc(portfolio, expenses, years)
			return fmt.Sprintf("%.2f", successRate)
		})
		check(err)
		firecalc_lookup := func(portfolio, expenses float64, years int) float64 {
			key := fmt.Sprintf("%.2f,%.2f,%d", portfolio, -expenses, years)
			value, _ := firecalc.Get(key, time.Minute*60*24*7)
			v, _ := strconv.ParseFloat(value, 64)
			return v
		}

		table = tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{
			"Year",
			"Age",
			"Portfolio",
			"Expenses",
			"X% Rule",
			"FIRECalc",
			fmt.Sprintf("PMT @ %.1f%%", (ror_retirement-inflation)*100),
		})
		for year := 2021; year < (1985 + life_expectancy); year++ {
			fv := FV(ror, float64(year-2021), -annual_contribution, -investmentTotal, false)
			expenses := expense_growth(year)
			age := year - 1985
			retirement_length := life_expectancy - age
			firecalc_success_rate := firecalc_lookup(fv, expenses, retirement_length)

			pmt := PMT(ror_retirement-inflation, float64(retirement_length), fv, 0.0, false)

			table.Append([]string{
				fmt.Sprintf("%d", year),
				fmt.Sprintf("%d", age),
				money(fv, true),
				money(expenses, true),
				fmt.Sprintf("%.1f%%", (-expenses/fv)*100),
				fmt.Sprintf("%.0f%%", firecalc_success_rate),
				fmt.Sprintf("%.1f%%", (pmt/-expenses)*100),
			})
		}
		table.Render()
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
		tsv := slice.GetEditCsv()
		// reload the slice
		// pdb2, err := NewPennyDb(*db, log, key)
		// check(err)
		// apply the edits
		check(slice.SaveEditCsv(bytes.NewReader(tsv)))
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

		err = pdb.Insert(importer.All())
		check(err)

		err = pdb.InsertInvestments(importer.AllInvestments())
		check(err)
	case list.FullCommand():
		slice.WriteHumanReadableTable(os.Stdout)
		fmt.Printf("\n\n")
		slice.WriteHumanReadableTotals(os.Stdout)
	case edit.FullCommand():
		tmpfile, err := ioutil.TempFile("", "")
		check(err)
		defer os.Remove(tmpfile.Name())

		_, err = tmpfile.Write(slice.GetEditCsv())
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
		slice.SaveEditCsv(bytes.NewReader(contents))
	}
}
