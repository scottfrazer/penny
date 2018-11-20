package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/leekchan/accounting"
	"io"
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
