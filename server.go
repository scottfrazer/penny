package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type RawFilter struct {
	Category string `json:"category"`
	Regex    string `json:"regex"`
	Start    string `json:"start"`
	End      string `json:"end"`
}

type Filter struct {
	Categories []string
	Regex      *regexp.Regexp
	Start      time.Time
	End        time.Time
}

func ParseFilter(raw RawFilter) (*Filter, map[string]string) {
	var filter Filter
	errors := make(map[string]string)

	filter.Categories = []string{}
	if len(raw.Category) > 0 {
		filter.Categories = strings.Split(raw.Category, ",")
	}

	regex, err := regexp.Compile(raw.Regex)
	if err != nil {
		errors["regex"] = err.Error()
	}
	filter.Regex = regex

	quarterRegex, err := regexp.Compile("Q(\\d)(\\d{4})")
	if err != nil {
		errors["regex"] = err.Error()
	}

	quarterRegexMatch := quarterRegex.FindStringSubmatch(raw.Start)
	if quarterRegexMatch != nil {
		quarter, _ := strconv.ParseInt(quarterRegexMatch[1], 10, 16)
		year, _ := strconv.ParseInt(quarterRegexMatch[2], 10, 16)
		start, end, err := quarterToDateRange(int(quarter), int(year))
		if err != nil {
			errors["start"] = err.Error()
		}
		filter.Start = start
		filter.End = end
	} else {
		if len(raw.Start) == 0 {
			filter.Start = time.Now().Add(-365 * 24 * time.Hour)
		} else {
			parsed, err := time.Parse("01/02/2006", raw.Start)
			if err != nil {
				errors["start"] = fmt.Sprintf("Invalid date: %s (expecting format MM/DD/YYYY)", raw.Start)
			}
			filter.Start = parsed
		}

		if len(raw.End) == 0 {
			filter.End = time.Now()
		} else {
			parsed, err := time.Parse("01/02/2006", raw.End)
			if err != nil {
				errors["end"] = fmt.Sprintf("Invalid date: %s (expecting format MM/DD/YYYY)", raw.End)
			}
			filter.End = parsed
		}
	}

	if len(errors) != 0 {
		return nil, errors
	} else {
		return &filter, nil
	}
}

func HttpGetTransactions(pdb *PennyDb) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Access-Control-Allow-Headers", "*")

		if req.Method == "OPTIONS" {
			return
		}

		body, err := ioutil.ReadAll(req.Body)
		check(err)

		var rawFilter RawFilter
		if len(body) > 0 {
			err = json.Unmarshal(body, &rawFilter)
			check(err)
		}

		filter, errors := ParseFilter(rawFilter)
		if len(errors) > 0 {
			w.WriteHeader(400)
			body, err := json.Marshal(errors)
			check(err)
			fmt.Fprintf(w, "%s", string(body))
			return
		}
		fmt.Printf("%v\n", filter)

		slice := pdb.Slice(filter)

		csvWriter := csv.NewWriter(w)
		for _, tx := range slice.transactions {
			csvWriter.Write([]string{
				tx.Source,
				tx.Date.Format("01/02/2006"),
				tx.Memo,
				fmt.Sprintf("%.2f", tx.Amount),
				tx.Disambiguation,
				tx.Category,
				fmt.Sprintf("%v", tx.Ignored),
			})
		}
		csvWriter.Flush()
		if err := csvWriter.Error(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}
	}
}

func HttpGetSummary(pdb *PennyDb) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Access-Control-Allow-Headers", "*")

		if req.Method == "OPTIONS" {
			return
		}

		body, err := ioutil.ReadAll(req.Body)
		check(err)

		var rawFilter RawFilter
		if len(body) > 0 {
			err = json.Unmarshal(body, &rawFilter)
			check(err)
		}

		filter, errors := ParseFilter(rawFilter)
		if len(errors) > 0 {
			w.WriteHeader(400)
			body, err := json.Marshal(errors)
			check(err)
			fmt.Fprintf(w, "%s", string(body))
			return
		}
		fmt.Printf("%v\n", filter)

		slice := pdb.Slice(filter)

		elapsedDays := slice.ElapsedDays()
		netTransactions := 0
		netAmount := 0.0

		csvWriter := csv.NewWriter(w)
		for _, summary := range slice.CategorySummaries() {
			netAmount += summary.Total
			netTransactions += summary.TransactionCount
			perDay := summary.Total / elapsedDays
			csvWriter.Write([]string{
				summary.Category,
				fmt.Sprintf("%d", summary.TransactionCount),
				fmt.Sprintf("%.02f", summary.Total),
				fmt.Sprintf("%.02f", perDay),
				fmt.Sprintf("%.02f", perDay*7),
				fmt.Sprintf("%.02f", perDay*30),
				fmt.Sprintf("%.2f%%", summary.PercentageOfIncome)})
		}

		netAmountPerDay := netAmount / elapsedDays
		csvWriter.Write([]string{
			"TOTAL",
			fmt.Sprintf("%d", netTransactions),
			fmt.Sprintf("%.02f", netAmount),
			fmt.Sprintf("%.02f", netAmountPerDay),
			fmt.Sprintf("%.02f", netAmountPerDay*7),
			fmt.Sprintf("%.02f", netAmountPerDay*30),
			""})

		csvWriter.Flush()
		if err := csvWriter.Error(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}
	}
}

func PennyHTTPServer(port string, pdb *PennyDb) {
	http.HandleFunc("/txs", HttpGetTransactions(pdb))
	http.HandleFunc("/summary", HttpGetSummary(pdb))
	fmt.Println("listening on :8090")
	http.ListenAndServe(":8090", nil)
}
