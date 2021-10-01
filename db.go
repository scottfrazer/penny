package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type dbExecFunc func(string, ...interface{}) (sql.Result, error)

type PennyDb struct {
	encryptedDbPath string
	secretKey       []byte
	mutex           *sync.RWMutex
	txCache         []*Transaction
	investmentCache []*Investment
	log             *Logger
}

type PennyDbHandle struct {
	db              *sql.DB
	pdb             *PennyDb
	readOnly        bool
	decryptedDbPath string
}

func NewPennyDb(encryptedDbPath string, log *Logger, secretKey []byte) (*PennyDb, error) {
	if len(secretKey) != 32 {
		return nil, fmt.Errorf("Expecting a secret key length of 32 bytes")
	}
	var mutex sync.RWMutex
	pdb := PennyDb{encryptedDbPath, secretKey, &mutex, nil, nil, log}

	handle, err := pdb.OpenReadOnly()
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	pdb.txCache, err = handle.AllTransactions()
	pdb.investmentCache, err = handle.AllInvestments()

	if err != nil {
		return nil, err
	}

	return &pdb, nil
}

func (pdb *PennyDb) AllInvestments() []*Investment {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()
	return pdb.investmentCache
}

func (pdb *PennyDb) AllTransactions() []*Transaction {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()
	return pdb.txCache
}

type PennyDbCache struct {
	table string
	fetch func(string) string
	pdb   *PennyDb
}

func (cache *PennyDbCache) Get(key string, ttl time.Duration) (string, error) {
	handle, err := cache.pdb.OpenReadWrite()
	if err != nil {
		return "", err
	}
	defer handle.Close()

	rows, err := handle.Query(fmt.Sprintf("SELECT value, date FROM %s WHERE key = ?", cache.table), key)
	if err != nil {
		return "", err
	}

	if rows.Next() {
		var value string
		var date time.Time
		err = rows.Scan(&value, &date)
		if err != nil {
			return "", err
		}

		err = rows.Err()
		if err != nil {
			return "", err
		}
		expiresAt := date.Add(ttl)

		if expiresAt.After(time.Now()) {
			rows.Close()
			return value, nil
		}
	}

	rows.Close()

	value := cache.fetch(key)

	res, err := handle.Exec(
		fmt.Sprintf(`INSERT OR REPLACE INTO %s (key, value, date) VALUES (?, ?, ?);`, cache.table),
		key,
		value,
		time.Now(),
	)

	if err != nil {
		return "", err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return "", err
	}

	if rowsAffected != 1 {
		return "", errors.New(fmt.Sprintf("could not update key %s", key))
	}

	return value, nil
}

func (pdb *PennyDb) DBBackedCache(table string, fetch func(string) string) (*PennyDbCache, error) {
	handle, err := pdb.OpenReadWrite()
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	_, err = handle.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		key TEXT PRIMARY KEY,
		value TEXT,
		date DATETIME
	);`, table))

	if err != nil {
		return nil, err
	}

	return &PennyDbCache{table, fetch, pdb}, nil
}

func (pdb *PennyDb) WriteDecryptedDb(outfile string) error {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()

	handle, err := pdb.OpenReadWrite()
	if err != nil {
		return err
	}
	defer handle.Close()

	data, err := ioutil.ReadFile(handle.decryptedDbPath)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(outfile, data, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (pdb *PennyDb) Slice(filter *Filter) *TxSlice {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()

	var sliceTxs []*Transaction
	for _, tx := range pdb.txCache {
		if len(filter.Categories) > 0 {
			found := false
			for _, category := range filter.Categories {
				if tx.Category == category || (category == "uncategorized" && len(tx.Category) == 0) {
					found = true
				}
			}
			if !found {
				continue
			}
		}

		if filter.Regex != nil && !filter.Regex.MatchString(strings.Join(tx.TableRow(), " ")) {
			continue
		}

		if tx.Date.Equal(filter.Start) || tx.Date.Equal(filter.End) || (tx.Date.After(filter.Start) && tx.Date.Before(filter.End)) {
			sliceTxs = append(sliceTxs, tx)
		}
	}

	return &TxSlice{sliceTxs, pdb}
}

func (pdb *PennyDb) DefaultSlice() *TxSlice {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()
	return &TxSlice{pdb.txCache, pdb}
}

func (pdb *PennyDb) Start() time.Time {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()
	return pdb.txCache[0].Date
}

func (pdb *PennyDb) End() time.Time {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()
	return pdb.txCache[len(pdb.txCache)-1].Date
}

func (pdb *PennyDb) Update(transactions []*Transaction) error {
	pdb.mutex.Lock()
	defer pdb.mutex.Unlock()

	handle, err := pdb.OpenReadWrite()
	if err != nil {
		return err
	}
	defer handle.Close()

	for _, tx := range transactions {
		res, err := handle.Exec(
			`UPDATE tx SET category=?, ignored=?, source=? WHERE date=? AND amount=? AND memo=? AND disambiguation=?`,
			tx.Category,
			tx.Ignored,
			tx.Source,
			tx.Date.Format("2006-01-02"),
			tx.Amount,
			tx.Memo,
			tx.Disambiguation)

		if err != nil {
			return err
		}

		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if rows != 1 {
			return fmt.Errorf("could not update transaction ID %s", tx.Id())
		}
	}

	pdb.txCache, err = handle.AllTransactions()
	if err != nil {
		return err
	}
	return nil
}

func (pdb *PennyDb) Insert(transactions []*Transaction) error {
	pdb.mutex.Lock()
	defer pdb.mutex.Unlock()

	handle, err := pdb.OpenReadWrite()
	if err != nil {
		return err
	}
	defer handle.Close()

	currentTransactions, err := handle.AllTransactions()
	if err != nil {
		return err
	}

	transactionFromId := make(map[string]*Transaction)
	for _, tx := range currentTransactions {
		transactionFromId[tx.Id()] = tx
	}

	for _, tx := range transactions {
		if _, ok := transactionFromId[tx.Id()]; ok {
			pdb.log.Info("Transaction with ID %s already in database", tx.Id())
			continue
		}

		res, err := handle.Exec(
			`INSERT INTO tx (source, date, amount, memo, disambiguation, category, ignored) values (?, ?, ?, ?, ?, ?, ?)`,
			tx.Source,
			tx.Date.Format("2006-01-02"),
			tx.Amount,
			tx.Memo,
			tx.Disambiguation,
			tx.Category,
			tx.Ignored)

		if err != nil {
			return err
		}

		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if rows != 1 {
			return errors.New("could not insert into 'tx' table")
		}
	}

	pdb.txCache, err = handle.AllTransactions()
	if err != nil {
		return err
	}

	return nil
}

func (pdb *PennyDb) InsertInvestments(investments []*Investment) error {
	pdb.mutex.Lock()
	defer pdb.mutex.Unlock()

	handle, err := pdb.OpenReadWrite()
	if err != nil {
		return err
	}
	defer handle.Close()

	currentInvestments, err := handle.AllInvestments()
	if err != nil {
		return err
	}

	investmentFromId := make(map[string]*Investment)
	for _, tx := range currentInvestments {
		investmentFromId[tx.Id()] = tx
	}

	for _, investment := range investments {
		if _, ok := investmentFromId[investment.Id()]; ok {
			pdb.log.Info("Investment with ID %s already in database", investment.Id())
			continue
		}

		res, err := handle.Exec(
			`INSERT INTO investment (account, date, type, symbol, shares, price, disambiguation) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			investment.Account,
			investment.Date.Format("2006-01-02"),
			investment.Type,
			investment.Symbol,
			investment.Shares,
			investment.Price,
			investment.Disambiguation)

		if err != nil {
			return err
		}

		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if rows != 1 {
			return errors.New("could not insert into 'investment' table")
		}
	}

	pdb.txCache, err = handle.AllTransactions()
	if err != nil {
		return err
	}

	return nil
}

func (pdb *PennyDb) decryptDbToTempFile() (string, error) {
	// Create temp file
	tmpfile, err := ioutil.TempFile("", "pennydb")
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(pdb.encryptedDbPath); os.IsNotExist(err) {
		_, err := sql.Open("sqlite3", tmpfile.Name())
		if err != nil {
			return "", err
		}
	} else {
		// Load encrypted sqlite3 database into memory
		start := time.Now()
		encryptedDbBytes, err := ioutil.ReadFile(pdb.encryptedDbPath)
		if err != nil {
			return "", err
		}
		pdb.log.Debug("read %s...  %s (%d bytes)", pdb.encryptedDbPath, time.Since(start), len(encryptedDbBytes))

		// Decrypt sqlite3 database
		start = time.Now()
		decryptedDbBytes, err := decrypt(pdb.secretKey, encryptedDbBytes)
		if err != nil {
			return "", err
		}
		pdb.log.Debug("decrypt %s...  %s (%d bytes)", pdb.encryptedDbPath, time.Since(start), len(decryptedDbBytes))

		// Write decrypted database to temp file
		start = time.Now()
		err = ioutil.WriteFile(tmpfile.Name(), decryptedDbBytes, 0664)
		if err != nil {
			return "", err
		}
		pdb.log.Debug("write decrypted sqlite3 db to %s...  %s", tmpfile.Name(), time.Since(start))
	}

	return tmpfile.Name(), nil
}

func (pdb *PennyDb) OpenReadWrite() (*PennyDbHandle, error) {
	return pdb.open(false)
}

func (pdb *PennyDb) OpenReadOnly() (*PennyDbHandle, error) {
	return pdb.open(true)
}

func (pdb *PennyDb) open(readOnly bool) (*PennyDbHandle, error) {
	path, err := pdb.decryptDbToTempFile()

	// Get Database handle
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	handle := &PennyDbHandle{db, pdb, readOnly, path}

	err = handle.Setup()

	if err != nil {
		return nil, err
	}

	return handle, nil
}

func (handle *PennyDbHandle) Close() error {
	handle.db.Close()

	// for read-only handles, don't save back the database
	if !handle.readOnly {
		dbBytes, err := ioutil.ReadFile(handle.decryptedDbPath)
		if err != nil {
			return err
		}

		start := time.Now()
		encryptedDbBytes, err := encrypt(handle.pdb.secretKey, dbBytes)
		if err != nil {
			return err
		}
		encryptTime := time.Since(start)
		handle.pdb.log.Debug("encrypt contents of %s...  %s", handle.decryptedDbPath, encryptTime)

		start = time.Now()
		err = ioutil.WriteFile(handle.pdb.encryptedDbPath, encryptedDbBytes, 0664)
		if err != nil {
			return err
		}
		writeTime := time.Since(start)
		handle.pdb.log.Debug("write encrypted sqlite3 db to %s...  %s", handle.pdb.encryptedDbPath, writeTime)
	}

	err := os.Remove(handle.decryptedDbPath)
	if err != nil {
		return err
	}
	return nil
}

func (handle *PennyDbHandle) Query(query string, args ...interface{}) (*sql.Rows, error) {
	handle.pdb.log.DbQuery(query, args...)
	return handle.db.Query(query, args...)
}

func (handle *PennyDbHandle) Exec(query string, args ...interface{}) (sql.Result, error) {
	handle.pdb.log.DbQuery(query, args...)
	return handle.db.Exec(query, args...)
}

func (handle *PennyDbHandle) AllInvestments() ([]*Investment, error) {
	rows, err := handle.Query("SELECT account, date, type, symbol, shares, price, disambiguation FROM investment ORDER BY date, account, shares, price, disambiguation;")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var investments []*Investment
	for rows.Next() {
		var investment Investment
		var date string
		err = rows.Scan(
			&investment.Account,
			&date,
			&investment.Type,
			&investment.Symbol,
			&investment.Shares,
			&investment.Price,
			&investment.Disambiguation,
		)
		if err != nil {
			return nil, err
		}
		investment.Date, err = time.Parse("2006-01-02", date)
		if err != nil {
			return nil, err
		}
		investments = append(investments, &investment)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return investments, nil
}

func (handle *PennyDbHandle) AllTransactions() ([]*Transaction, error) {
	rows, err := handle.Query("SELECT source, date, amount, memo, disambiguation, category, ignored FROM tx ORDER BY date, amount, memo, disambiguation;")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []*Transaction
	for rows.Next() {
		var tx Transaction
		var date string
		err = rows.Scan(&tx.Source, &date, &tx.Amount, &tx.Memo, &tx.Disambiguation, &tx.Category, &tx.Ignored)
		if err != nil {
			return nil, err
		}
		tx.Date, err = time.Parse("2006-01-02", date)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, &tx)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return transactions, nil
}

func (handle *PennyDbHandle) Setup() error {
	rows, err := handle.Query("SELECT name FROM sqlite_master WHERE type='table';")
	if err != nil {
		return err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			return err
		}
		tables = append(tables, name)
	}

	err = rows.Err()
	if err != nil {
		return err
	}

	if !contains("tx", tables) {
		_, err := handle.Exec(`CREATE TABLE tx (
			source TEXT,
			date TEXT,
			memo TEXT,
			amount FLOAT,
			disambiguation TEXT,
			category TEXT,
			ignored INTEGER
		);`)

		if err != nil {
			return err
		}
	}

	if !contains("investment", tables) {
		_, err := handle.Exec(`CREATE TABLE investment (
			account INTEGER,
			date TEXT,
			type TEXT,
			symbol TEXT,
			shares FLOAT,
			price FLOAT,
			disambiguation TEXT
		);`)

		if err != nil {
			return err
		}
	}
	return nil
}
