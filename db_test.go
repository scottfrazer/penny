package main

import (
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
		if expectedTx.Id() != actualTx.Id() {
			t.Fatalf("Expecting transaction ID to be %s, got %s", expectedTx.Id(), actualTx.Id())
		}
		if expectedTx.Category != actualTx.Category {
			t.Fatalf("Expecting transaction category to be %s, got %s", expectedTx.Category, actualTx.Category)
		}
		if expectedTx.Ignored != actualTx.Ignored {
			t.Logf("expected:\n%s\nactual:\n%s\n", expectedTx.String(), actualTx.String())
			t.Fatalf("Expecting transaction ignored flag to be %v, got %v", expectedTx.Ignored, actualTx.Ignored)
		}
		if expectedTx.Disambiguation != actualTx.Disambiguation {
			t.Fatalf("Expecting transaction disambiguation string to be %v, got %v", expectedTx.Disambiguation, actualTx.Disambiguation)
		}
	}
}

func fail(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Fatal error: %v", err)
	}
}

func TestDatabase(t *testing.T) {
	log := NewLogger().ToWriter(os.Stdout)

	// Start with no database file
	file, err := ioutil.TempFile("", "test")
	fail(t, err)
	os.Remove(file.Name())

	// Initialize database from nothing
	pdb, err := NewPennyDb(file.Name(), log, []byte("01234567890123456789012345678901"))
	fail(t, err)

	time1, _ := time.Parse("Jan 2 2006", "Jan 1 2018")
	time2, _ := time.Parse("Jan 2 2006", "Jan 2 2018")
	time3, _ := time.Parse("Jan 2 2006", "Jan 3 2018")
	time4, _ := time.Parse("Jan 2 2006", "Jan 4 2018")
	tx1 := Transaction{"source", time1, "memo", 1.1, "", "category1", false}
	tx1_mod := Transaction{"source", time1, "memo", 1.1, "", "new_category", true}
	tx2 := Transaction{"source2", time2, "memo2", 1.2, "", "category2", false}
	tx2_mod := Transaction{"source2", time2, "memo2", 1.2, "", "category2_new", false}
	tx3 := Transaction{"source3", time3, "memo3", 1.3, "", "category3", false}
	tx3_mod := Transaction{"source3", time3, "memo3", 1.3, "", "category3_new", true}
	tx4 := Transaction{"source4", time4, "memo4", 1.4, "disambiguation", "category4", false}

	// Insert a transaction
	err = pdb.Insert([]*Transaction{&tx1})
	fail(t, err)

	assertTransactions(t, []*Transaction{&tx1}, pdb.AllTransactions())

	// Insert the same transaction a second time, it should be rejected
	err = pdb.Insert([]*Transaction{&tx1})
	fail(t, err)

	assertTransactions(t, []*Transaction{&tx1}, pdb.AllTransactions())

	// Update the category and ignored flag for tx1
	err = pdb.Update([]*Transaction{&tx1_mod})
	fail(t, err)

	assertTransactions(t, []*Transaction{&tx1_mod}, pdb.AllTransactions())

	// Insert a transaction
	err = pdb.Insert([]*Transaction{&tx2})
	fail(t, err)

	assertTransactions(t, []*Transaction{&tx1_mod, &tx2}, pdb.AllTransactions())

	// Insert multiple transactions
	err = pdb.Insert([]*Transaction{&tx3, &tx4})
	fail(t, err)

	assertTransactions(t, []*Transaction{&tx1_mod, &tx2, &tx3, &tx4}, pdb.AllTransactions())

	// Generate an Edit TSV for all transactions
	actualEditTsv := string(pdb.DefaultSlice().GetEditTsv())

	expectedEditTsv := `e14c27b5be,memo,01/01/2018,$1.10,true,new_category
4d6e62d4a0,memo2,01/02/2018,$1.20,false,category2
82d91ccd30,memo3,01/03/2018,$1.30,false,category3
1fe6dfb8ab,memo4,01/04/2018,$1.40,false,category4
`
	if actualEditTsv != expectedEditTsv {
		t.Fatalf("Expected edit TSV:\n`%s`\nActual edit TSV:\n`%s`\n", expectedEditTsv, actualEditTsv)
	}

	editTsv := `e14c27b5be,memo,01/01/2018,$1.10,true,new_category
4d6e62d4a0,memo2,01/02/2018,$1.20,false,category2_new
82d91ccd30,memo3,01/03/2018,$1.30,true,category3_new
1fe6dfb8ab,memo4,01/04/2018,$1.40,false,category4
`
	err = pdb.SaveEditTsv([]byte(editTsv))
	fail(t, err)

	assertTransactions(t, []*Transaction{&tx1_mod, &tx2_mod, &tx3_mod, &tx4}, pdb.AllTransactions())

	// Clean up
	os.Remove(file.Name())
}
