package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/leekchan/accounting"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func PadRight(str, pad string, length int) string {
	for {
		str += pad
		if len(str) > length {
			return str[0:length]
		}
	}
}

func contains(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

var ac = accounting.Accounting{Symbol: "$", Precision: 2}

func money(amount float64, color bool) string {
	colorFunc := nocolor
	if color {
		if amount > 0 {
			colorFunc = green
		} else {
			colorFunc = red
		}
	}
	return colorFunc(ac.FormatMoney(amount))
}

func red(s string) string {
	return fmt.Sprintf("\033[38;5;1m%s\033[0m", s)
}

func green(s string) string {
	return fmt.Sprintf("\033[38;5;2m%s\033[0m", s)
}

func nocolor(s string) string {
	return s
}

func encrypt(key, text []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	ciphertext := make([]byte, aes.BlockSize+len(text))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	cfb := cipher.NewCFBEncrypter(block, iv)
	cfb.XORKeyStream(ciphertext[aes.BlockSize:], text)
	return ciphertext, nil
}

func decrypt(key, text []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(text) < aes.BlockSize {
		return nil, errors.New("ciphertext too short")
	}
	iv := text[:aes.BlockSize]
	text = text[aes.BlockSize:]
	cfb := cipher.NewCFBDecrypter(block, iv)
	cfb.XORKeyStream(text, text)
	return text, nil
}

func monthToQuarter(month int) int {
	if month >= 1 && month <= 3 {
		return 1
	} else if month >= 4 && month <= 6 {
		return 2
	} else if month >= 7 && month <= 9 {
		return 3
	} else if month >= 10 && month <= 12 {
		return 4
	}
	return 0
}

func quarterToDateRange(quarter, year int) (time.Time, time.Time, error) {
	var startMonth, endMonth, endDay int
	switch quarter {
	case 1:
		startMonth = 1
		endMonth = 3
		endDay = 31
	case 2:
		startMonth = 4
		endMonth = 6
		endDay = 30
	case 3:
		startMonth = 7
		endMonth = 9
		endDay = 30
	case 4:
		startMonth = 10
		endMonth = 12
		endDay = 31
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("Invalid quarter")
	}

	start, err := time.Parse("01/02/2006", fmt.Sprintf("%02d/01/%d", startMonth, year))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("Invalid quarter")
	}

	end, err := time.Parse("01/02/2006", fmt.Sprintf("%02d/%02d/%d", endMonth, endDay, year))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("Invalid quarter")
	}

	return start, end, nil
}
