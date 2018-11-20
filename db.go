package main

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type dbExecFunc func(string, ...interface{}) (sql.Result, error)

type PennyDb struct {
	encryptedDbPath string
	secretKey       []byte
	mutex           *sync.RWMutex
	txCache         []*Transaction
	log             *Logger
}

type PennyDbHandle struct {
	db              *sql.DB
	pdb             *PennyDb
	decryptedDbPath string
}

func NewPennyDb(encryptedDbPath string, log *Logger, secretKey []byte) (*PennyDb, error) {
	if len(secretKey) != 32 {
		return nil, fmt.Errorf("Expecting a secret key length of 32 bytes")
	}
	var mutex sync.RWMutex
	pdb := PennyDb{encryptedDbPath, secretKey, &mutex, nil, log}

	handle, err := pdb.Open()
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	pdb.txCache, err = handle.AllTransactions()
	if err != nil {
		return nil, err
	}

	return &pdb, nil
}

func (pdb *PennyDb) AllTransactions() []*Transaction {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()
	return pdb.txCache
}

func (pdb *PennyDb) WriteDecryptedDb(outfile string) error {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()

	handle, err := pdb.Open()
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

func (pdb *PennyDb) SaveEditTsv(contents []byte) error {
	r := csv.NewReader(bytes.NewReader(contents))
	records, err := r.ReadAll()
	if err != nil {
		return err
	}

	txs := make(map[string]*Transaction)
	for _, tx := range pdb.txCache {
		txs[tx.Id()] = tx
	}

	var changed []*Transaction
	for _, record := range records {
		id := record[0]
		ignore := false
		if record[4] == "true" {
			ignore = true
		}
		category := record[5]
		if tx, ok := txs[id]; ok {
			isChanged := tx.Category != category || tx.Ignored != ignore
			if isChanged {
				tx.Category = category
				tx.Ignored = ignore
				changed = append(changed, tx)
			}
		} else {
			fmt.Printf("Error: cannot find id %s\n", id)
		}
	}

	err = pdb.Update(changed)
	return err
}

func (pdb *PennyDb) Slice(start, end time.Time, regex *regexp.Regexp, categories []string) *TxSlice {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()

	var sliceTxs []*Transaction
	for _, tx := range pdb.txCache {
		if len(categories) > 0 {
			found := false
			for _, category := range categories {
				if tx.Category == category || (category == "uncategorized" && len(tx.Category) == 0) {
					found = true
				}
			}
			if !found {
				continue
			}
		}

		if regex != nil && !regex.MatchString(strings.Join(tx.TableRow(), " ")) {
			continue
		}

		if tx.Date.Equal(start) || tx.Date.Equal(end) || (tx.Date.After(start) && tx.Date.Before(end)) {
			sliceTxs = append(sliceTxs, tx)
		}
	}
	return &TxSlice{sliceTxs}
}

func (pdb *PennyDb) DefaultSlice() *TxSlice {
	pdb.mutex.RLock()
	defer pdb.mutex.RUnlock()
	return &TxSlice{pdb.txCache}
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

	handle, err := pdb.Open()
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
			return errors.New(fmt.Sprintf("could not update transaction ID %s", tx.Id()))
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

	handle, err := pdb.Open()
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

func (handle *PennyDbHandle) Close() error {
	handle.db.Close()

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

	err = os.Remove(handle.decryptedDbPath)
	if err != nil {
		return err
	}
	return nil
}

func (pdb *PennyDb) Open() (*PennyDbHandle, error) {
	// Create temp file
	tmpfile, err := ioutil.TempFile("", "pennydb")
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(pdb.encryptedDbPath); os.IsNotExist(err) {
		// Encypted SQLite file does not exist

		db, err := sql.Open("sqlite3", tmpfile.Name())
		if err != nil {
			return nil, err
		}

		handle := &PennyDbHandle{db, pdb, tmpfile.Name()}

		err = handle.Setup()
		if err != nil {
			return nil, err
		}

	} else {
		// Encrypted SQLite file does exist

		// Load encrypted sqlite3 database into memory
		encryptedDbBytes, err := ioutil.ReadFile(pdb.encryptedDbPath)
		if err != nil {
			return nil, err
		}

		// Decrypt sqlite3 database
		start := time.Now()
		decryptedDbBytes, err := decrypt(pdb.secretKey, encryptedDbBytes)
		if err != nil {
			return nil, err
		}
		decryptTime := time.Since(start)
		pdb.log.Debug("decrypt contents of %s...  %s (%d bytes)", pdb.encryptedDbPath, decryptTime, len(decryptedDbBytes))

		// Write decrypted database to temp file
		start = time.Now()
		err = ioutil.WriteFile(tmpfile.Name(), decryptedDbBytes, 0664)
		if err != nil {
			return nil, err
		}
		writeTime := time.Since(start)
		pdb.log.Debug("write decrypted sqlite3 db to %s...  %s", tmpfile.Name(), writeTime)
	}

	// Get Database handle
	db, err := sql.Open("sqlite3", tmpfile.Name())
	if err != nil {
		return nil, err
	}

	return &PennyDbHandle{db, pdb, tmpfile.Name()}, nil
}

func (handle *PennyDbHandle) Query(query string, args ...interface{}) (*sql.Rows, error) {
	handle.pdb.log.DbQuery(query, args...)
	return handle.db.Query(query, args...)
}

func (handle *PennyDbHandle) Exec(query string, args ...interface{}) (sql.Result, error) {
	handle.pdb.log.DbQuery(query, args...)
	return handle.db.Exec(query, args...)
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
	return nil
}
