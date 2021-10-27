package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Investment struct {
	Account        int64
	Date           time.Time
	Type           string
	Symbol         string
	Shares         float64
	Price          float64
	Disambiguation string
}

type Holding struct {
	Account     int64
	Symbol      string
	Investments []*Investment
}

type HoldingAccountAndSymbolSort []*Holding

func (holdings HoldingAccountAndSymbolSort) Len() int {
	return len(holdings)
}

func (holdings HoldingAccountAndSymbolSort) Swap(i, j int) {
	holdings[i], holdings[j] = holdings[j], holdings[i]
}

func (holdings HoldingAccountAndSymbolSort) Less(i, j int) bool {
	return strings.Compare(holdings[i].Key(), holdings[j].Key()) < 0
}

func (holding *Holding) Shares() float64 {
	var total float64
	for _, investment := range holding.Investments {
		total += investment.Shares
	}
	return total
}

func (holding *Holding) PurchasePrice() float64 {
	var total float64
	for _, investment := range holding.Investments {
		total += investment.Price * investment.Shares
	}
	return total
}

func (holding *Holding) CurrentPrice(lookup *StockSymbolLookup) (float64, error) {
	var total float64
	price, err := lookup.Get(holding.Symbol)
	if err != nil {
		return 0.0, err
	}
	for _, investment := range holding.Investments {
		total += price * investment.Shares
	}
	return total, nil
}

func (holding *Holding) Key() string {
	return fmt.Sprintf("%d-%s", holding.Account, holding.Symbol)
}

func (investment *Investment) Id() string {
	hasher := md5.New()
	concat := fmt.Sprintf(
		"%d%s%s%s%.2f%.2f%s",
		investment.Account,
		investment.Date.Format("01/02/2006"),
		investment.Type,
		investment.Symbol,
		investment.Shares,
		investment.Price,
		investment.Disambiguation,
	)
	hasher.Write([]byte(concat))
	return hex.EncodeToString(hasher.Sum(nil))[:10]
}

func lookupStockPrice(symbol string) (float64, error) {
	if symbol == "401K" {
		return 1.0, nil
	}

	type YahooFinanceResponse struct {
		OptionChain struct {
			Result []struct {
				Quote struct {
					RegularMarketPrice float64
				} `json:"quote"`
			} `json:"result"`
		} `json:"optionChain"`
	}

	resp, err := http.Get(fmt.Sprintf("https://query1.finance.yahoo.com/v7/finance/options/%s", symbol))
	if err != nil {
		return 0.0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0.0, err
	}
	var response YahooFinanceResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0.0, err
	}
	return response.OptionChain.Result[0].Quote.RegularMarketPrice, nil
}

type StockSymbolLookup struct {
	cache *PennyDbCache
	ttl   time.Duration
}

func NewStockSymbolLookup(pdb *PennyDb) (*StockSymbolLookup, error) {
	cache, err := pdb.DBBackedCache("stocks", func(symbol string) string {
		price, err := lookupStockPrice(symbol)
		check(err)
		return fmt.Sprintf("%.2f", price)
	})

	if err != nil {
		return nil, err
	}

	return &StockSymbolLookup{cache, time.Hour * 24}, nil
}

func (lookup *StockSymbolLookup) Get(symbol string) (float64, error) {
	value, err := lookup.cache.Get(symbol, lookup.ttl)
	if err != nil {
		return 0, err
	}
	price, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	return price, nil
}
