package main

import (
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestImport(t *testing.T) {
	time1, _ := time.Parse("Jan 2 2006", "Jan 1 2018")
	time2, _ := time.Parse("Jan 2 2006", "Jan 2 2018")
	time3, _ := time.Parse("Jan 2 2006", "Jan 3 2018")
	time4, _ := time.Parse("Jan 2 2006", "Jan 4 2018")
	tx1 := Transaction{"dcu", time1, "memo", -1.1, "", "", false}
	tx2 := Transaction{"dcu2", time2, "memo2", -1.2, "", "", false}
	tx3 := Transaction{"dcu3", time3, "memo3", -1.3, "", "", false}
	tx4 := Transaction{"chase", time4, "memo4", -1.4, "", "", false}

	dcuImportFile := `"DATE","DESCRIPTION","AMOUNT","CURRENT BALANCE"
"01/01/2018","memo","-1.1","998.9"`

	dcu2ImportFile := `"DATE","DESCRIPTION","AMOUNT","CURRENT BALANCE"
"01/02/2018","memo2","-1.2","998.8"`

	dcu3ImportFile := `"DATE","DESCRIPTION","AMOUNT","CURRENT BALANCE"
"01/03/2018","memo3","-1.3","998.7"`

	chaseImportFile := `Transaction Date,Post Date,Description,Category,Type,Amount,Memo
01/04/2018,01/04/2018,memo4,Food & Drink,Sale,-1.4,`

	importer := NewTransactionImporter()
	importer.ImportDCU("dcu", []byte(dcuImportFile))
	importer.ImportDCU("dcu2", []byte(dcu2ImportFile))
	importer.ImportDCU("dcu3", []byte(dcu3ImportFile))
	importer.ImportAmazonRewards("chase", []byte(chaseImportFile))

	file, err := ioutil.TempFile("", "test")
	fail(t, err)
	os.Remove(file.Name())

	pdb, err := NewPennyDb(file.Name(), NewLogger() /*.ToWriter(os.Stdout)*/, []byte("01234567890123456789012345678901"))
	fail(t, err)
	err = pdb.LoadCaches()
	fail(t, err)

	err = pdb.Insert(importer.All())
	fail(t, err)

	assertTransactions(t, []*Transaction{&tx1, &tx2, &tx3, &tx4}, pdb.AllTransactions())
}
