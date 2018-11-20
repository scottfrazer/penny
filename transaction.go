package main

import (
	"bytes"
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"github.com/olekukonko/tablewriter"
	"io"
	"math"
	"sort"
	"time"
)

type Transaction struct {
	Source         string
	Date           time.Time
	Memo           string
	Amount         float64
	Disambiguation string
	Category       string
	Ignored        bool
}

type TransactionDateSort []*Transaction

func (tds TransactionDateSort) Len() int {
	return len(tds)
}

func (tds TransactionDateSort) Swap(i, j int) {
	tds[i], tds[j] = tds[j], tds[i]
}

func (tds TransactionDateSort) Less(i, j int) bool {
	if tds[i].Date.Format("01/02/2016") != tds[j].Date.Format("01/02/2016") {
		return tds[i].Date.Before(tds[j].Date)
	} else if tds[i].Id() != tds[j].Id() {
		return tds[i].Id() < tds[j].Id()
	} else {
		panic(fmt.Sprintf("Two transactions are equal: %s %s", tds[i], tds[j]))
	}
}

func (tx *Transaction) Id() string {
	hasher := md5.New()
	concat := fmt.Sprintf("%s%.2f%s%s", tx.Date.Format("01/02/2006"), tx.Amount, tx.Memo, tx.Disambiguation)
	hasher.Write([]byte(concat))
	return hex.EncodeToString(hasher.Sum(nil))[:10]
}

func (tx *Transaction) String() string {
	return fmt.Sprintf(
		"%s %s %.2f %s %s %s %v", tx.Id(), tx.Date.Format("01/02/2006"), tx.Amount, tx.Memo, tx.Source, tx.Category, tx.Ignored,
	)
}

func (tx *Transaction) TableRow() []string {
	ignored := "✓"
	if tx.Ignored {
		ignored = "✘"
	}
	return []string{ignored, tx.Source, tx.Date.Format("01/02/2006"), money(tx.Amount, false), tx.Category, tx.Memo}
}

func (tx *Transaction) CsvRow() []string {
	return []string{
		tx.Source,
		tx.Date.Format("01/02/2006"),
		tx.Memo,
		fmt.Sprintf("%.2f", tx.Amount),
		tx.Disambiguation,
	}
}

type TxSlice struct {
	transactions []*Transaction // assume sorted
}

func (slice *TxSlice) ElapsedDays() float64 {
	if len(slice.transactions) == 0 {
		return 0
	}
	start := slice.transactions[0].Date
	end := slice.transactions[len(slice.transactions)-1].Date
	days := end.Sub(start).Hours() / 24
	if days < 1 {
		return 1
	}
	return days
}

func (slice *TxSlice) Start() time.Time {
	return slice.transactions[0].Date
}

func (slice *TxSlice) End() time.Time {
	return slice.transactions[len(slice.transactions)-1].Date
}

func (slice *TxSlice) Total() float64 {
	var total float64 = 0
	for _, tx := range slice.transactions {
		if !tx.Ignored {
			total += tx.Amount
		}
	}
	return total
}

func (slice *TxSlice) GetEditTsv() []byte {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	for _, tx := range slice.transactions {
		row := []string{
			tx.Id(),
			tx.Memo,
			tx.Date.Format("01/02/2006"),
			money(tx.Amount, false),
			fmt.Sprintf("%v", tx.Ignored),
			tx.Category,
		}
		err := writer.Write(row)
		check(err)
	}
	writer.Flush()
	check(writer.Error())
	return buf.Bytes()
}

func (slice *TxSlice) Categories() []string {
	categories := make(map[string]struct{})
	for _, tx := range slice.transactions {
		if tx.Ignored {
			continue
		}
		categories[tx.Category] = struct{}{}
	}

	categoriesSlice := make([]string, len(categories))
	index := 0
	for category, _ := range categories {
		categoriesSlice[index] = category
		index++
	}
	return categoriesSlice
}

func (slice *TxSlice) TransactionCountByCategory() map[string]int {
	totals := make(map[string]int)
	for _, tx := range slice.transactions {
		if tx.Ignored {
			continue
		}
		totals[tx.Category] += 1
	}
	return totals
}

type CategorySummary struct {
	Category           string
	Total              float64
	TransactionCount   int
	PercentageOfIncome float64
}

type CategorySummarySortAmountDescending []CategorySummary

func (arr CategorySummarySortAmountDescending) Len() int {
	return len(arr)
}

func (arr CategorySummarySortAmountDescending) Swap(i, j int) {
	arr[i], arr[j] = arr[j], arr[i]
}

func (arr CategorySummarySortAmountDescending) Less(i, j int) bool {
	return math.Abs(arr[i].Total) > math.Abs(arr[j].Total)
}

func (slice *TxSlice) CategorySummaries() []CategorySummary {
	income := 0.0
	for _, tx := range slice.transactions {
		if tx.Category == "income" {
			income += tx.Amount
		}
	}

	totalByCategory := make(map[string]float64)
	transactionCountByCategory := make(map[string]int)
	for _, tx := range slice.transactions {
		if tx.Ignored {
			continue
		}
		totalByCategory[tx.Category] += tx.Amount
		transactionCountByCategory[tx.Category] += 1
	}

	result := make([]CategorySummary, len(totalByCategory))
	index := 0
	for category, total := range totalByCategory {
		transactionCount := transactionCountByCategory[category]
		percentOfIncome := float64(0)
		if income > 0 {
			percentOfIncome = math.Abs(total) / income * 100
		}
		result[index] = CategorySummary{category, total, transactionCount, percentOfIncome}
		index++
	}
	sort.Sort(CategorySummarySortAmountDescending(result))
	return result
}

func (slice *TxSlice) TableRows(color bool) []string {
	maxColumnWidth := make(map[int]int)
	for _, tx := range slice.transactions {
		for col, cell := range tx.TableRow() {
			currentMax, ok := maxColumnWidth[col]
			if !ok || len(cell) > currentMax {
				maxColumnWidth[col] = len(cell)
			}
		}
	}

	var table []string
	for _, tx := range slice.transactions {
		var colors = map[int]func(s string) string{
			0: nocolor,
			1: nocolor,
			2: nocolor,
			3: nocolor,
			4: nocolor,
			5: nocolor,
		}

		if color {
			if tx.Amount > 0 {
				colors[3] = green
			} else {
				colors[3] = red
			}
		}

		var rowString = ""
		for i, cell := range tx.TableRow() {
			rowString += colors[i](PadRight(cell, " ", maxColumnWidth[i]+1))
		}

		table = append(table, rowString)
	}
	return table
}

func (slice *TxSlice) WriteHumanReadableTable(writer io.Writer) {
	for _, row := range slice.TableRows(true) {
		io.WriteString(writer, row)
		io.WriteString(writer, "\n")
	}
}

func (slice *TxSlice) WriteHumanReadableTotals(writer io.Writer) {
	elapsedDays := slice.ElapsedDays()

	table := tablewriter.NewWriter(writer)
	table.Append([]string{"First Transaction", slice.transactions[0].Date.Format("01/02/2006")})
	table.Append([]string{"Last Transaction", slice.transactions[len(slice.transactions)-1].Date.Format("01/02/2006")})
	table.Append([]string{"Elapsed Days", fmt.Sprintf("%d", int(elapsedDays))})
	table.Append([]string{"Transaction Count", fmt.Sprintf("%d", len(slice.transactions))})
	table.Render()

	io.WriteString(writer, "\n")

	netTransactions := 0
	netAmount := 0.0

	table = tablewriter.NewWriter(writer)
	table.SetHeader([]string{"Category", "#", "Total", "Per Day", "Per Week", "Per Month", "% Income"})
	table.SetBorder(false)

	for _, summary := range slice.CategorySummaries() {
		netAmount += summary.Total
		netTransactions += summary.TransactionCount
		perDay := summary.Total / elapsedDays
		table.Append([]string{
			summary.Category,
			fmt.Sprintf("%d", summary.TransactionCount),
			money(summary.Total, true),
			money(perDay, true),
			money(perDay*7, true),
			money(perDay*30, true),
			fmt.Sprintf("%.2f%%", summary.PercentageOfIncome)})
	}

	netAmountPerDay := netAmount / elapsedDays
	footer := []string{
		"TOTAL",
		fmt.Sprintf("%d", netTransactions),
		money(netAmount, true),
		money(netAmountPerDay, true),
		money(netAmountPerDay*7, true),
		money(netAmountPerDay*30, true),
		""}
	table.Append(footer)

	table.Render()
}
