package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func assertTransactions(t *testing.T, expected []*Transaction, actual []*Transaction) {
	if len(expected) != len(actual) {
		t.Fatalf("Expecting %d transactions, got %d", len(expected), len(actual))
	}

	for index, _ := range expected {
		expectedTx := expected[index]
		actualTx := actual[index]
		if !actualTx.Equals(expectedTx) {
			t.Fatalf("Expected actual %v to be %v", actualTx, expectedTx)
		}
	}
}

func fail(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Fatal error: %v", err)
	}
}

func TestDatabase(t *testing.T) {
	dbPath := tempFilePath()
	defer os.Remove(dbPath)

	pdb, err := NewPennyDb(dbPath, NewLogger(), []byte("01234567890123456789012345678901"))
	fail(t, err)

	tx1 := Transaction{"source", date("Jan 1 2018"), "memo", 1.1, "", "category1", false}
	tx1_mod := Transaction{"source", date("Jan 1 2018"), "memo", 1.1, "", "category1_NEW", true}
	tx2 := Transaction{"source2", date("Jan 2 2018"), "memo2", 1.2, "", "category2", false}
	tx2_mod := Transaction{"source2", date("Jan 2 2018"), "memo2", 1.2, "", "category2_NEW", false}
	tx3 := Transaction{"source3", date("Jan 3 2018"), "memo3", 1.3, "", "category3", false}
	tx3_mod := Transaction{"source3", date("Jan 3 2018"), "memo3", 1.3, "", "category3_NEW", true}
	tx4 := Transaction{"source4", date("Jan 4 2018"), "memo4", 1.4, "disambiguation", "category4", false}

	first := []*Transaction{&tx1, &tx2, &tx3, &tx4}

	err = pdb.Insert(first)
	fail(t, err)

	assertTransactions(t, first, pdb.AllTransactions())

	second := []*Transaction{&tx1_mod, &tx2, &tx3, &tx4}

	err = pdb.Update(second)
	fail(t, err)

	assertTransactions(t, second, pdb.AllTransactions())

	csv, err := getEditCsv(second)
	fail(t, err)

	txs, err := parseEditCsv(csv, pdb.AllTransactions())
	fail(t, err)

	assertTransactions(t, txs, []*Transaction{})

	third := []*Transaction{&tx1_mod, &tx2_mod, &tx3_mod, &tx4}

	csv, err = getEditCsv(third)
	fail(t, err)

	txs, err = parseEditCsv(csv, pdb.AllTransactions())
	fail(t, err)

	assertTransactions(t, txs, []*Transaction{&tx2_mod, &tx3_mod})
}

func TestDbBackedCache(t *testing.T) {
	dbPath := tempFilePath()
	defer os.Remove(dbPath)

	pdb, err := NewPennyDb(dbPath, NewLogger(), []byte("01234567890123456789012345678901"))
	fail(t, err)

	lookupFuncCalled := false

	cache, err := pdb.DBBackedCache("cache_table", func(key string) (string, error) {
		lookupFuncCalled = true
		switch key {
		case "a":
			return "a_value", nil
		case "b":
			return "", fmt.Errorf("an error has occurred")
		}
		return "", nil
	})
	fail(t, err)

	lookupFuncCalled = false
	_, err = cache.Get("b")
	if err == nil {
		t.Fatalf("expecting error")
	}
	if lookupFuncCalled == false {
		t.Fatalf("expected func to be called")
	}

	lookupFuncCalled = false
	value, err := cache.Get("foo")
	fail(t, err)
	if value != "" {
		t.Fatalf("expecting no value")
	}
	if lookupFuncCalled == false {
		t.Fatalf("expected func to be called")
	}

	lookupFuncCalled = false
	value, err = cache.Get("a")
	fail(t, err)
	if value != "a_value" {
		t.Fatalf("expecting mapping of a -> a_value")
	}
	if lookupFuncCalled == false {
		t.Fatalf("expected func to be called")
	}

	lookupFuncCalled = false
	value, err = cache.GetWithTTL("a", time.Hour)
	fail(t, err)
	if value != "a_value" {
		t.Fatalf("expecting mapping of a -> a_value")
	}
	if lookupFuncCalled == true {
		t.Fatalf("expected lookup function to not be called")
	}

	time.Sleep(time.Millisecond * 500)

	lookupFuncCalled = false
	value, err = cache.GetWithTTL("a", time.Millisecond*200)
	fail(t, err)
	if value != "a_value" {
		t.Fatalf("expecting mapping of a -> a_value")
	}
	if lookupFuncCalled == false {
		t.Fatalf("expected func to be called")
	}
}

func tempFilePath() string {
	file, err := ioutil.TempFile("", "penny")
	check(err)
	os.Remove(file.Name())
	return file.Name()
}

func date(s string) time.Time {
	t, err := time.Parse("Jan 2 2006", s)
	check(err)
	return t
}
