package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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

func FV(rate, nper, pmt, pv float64, beginning bool) float64 {
	return (-pv * math.Pow(1+rate, nper)) + (-pmt * (math.Pow(1+rate, nper) - 1) / rate)
}

func PMT(rate, nper, pv, fv float64, beginning bool) float64 {
	return (pv * rate) / (1 - math.Pow(1+rate, -nper))
}

func FIRECalc(portfolio, expenses float64, years int) (float64, error) {
	// POST https://www.firecalc.com/firecalcresults.php
	//
	// wdamt=175000&PortValue=5200000&term=35&callprocess2=Submit&ss1=0&ssy1=2034&ss2=0&ssy2=2036&signwd1=%2B&chwd1=0&chyr1=2024&wd1infl=adj&signwd2=%2B&chwd2=0&chyr2=2026&wd2infl=adj&signwd3=%2B&chwd3=0&chyr3=2030&wd3infl=adj&holdyears=2021&preadd=0&inflpick=4&override_inflation_rate=3.0&SpendingModel=constant&age=48%09&pctlastyear=0%09&infltype=PPI&fixedinc=Commercial+Paper&user_bonds=0&InvExp=0.18&monte=history&StartYr=1871&fixedchoice=LongInterest&pctEquity=75&mix1=+10+++&mix2=+10+++&mix3=+10+++&mix4=+40+++&mix5=+40+++&mix6=+10+++&mix7=+15+++&mix8=+5+++&user_bonds=4.0&user_inflation=3.0&monte_growth=10&monte_sd=10&monte_inflation=3.00&signlump1=%2B&cashin1=0&cashyr1=2024&signlump2=%2B&cashin2=0&cashyr2=2034&signlump3=%2B&cashin3=0&cashyr3=2039&process=survival&showyear=1960%09%09&delay=10&goal=95&portfloor=0&FIRECalcVersion=3.0

	query := make(map[string]string)
	query["wdamt"] = fmt.Sprintf("%.2f", expenses)
	query["PortValue"] = fmt.Sprintf("%.2f", portfolio)
	query["term"] = fmt.Sprintf("%d", years)
	query["callprocess2"] = "Submit"
	query["ss1"] = "0"
	query["ssy1"] = "2034"
	query["ss2"] = "0"
	query["ssy2"] = "2036"
	query["signwd1"] = "+"
	query["chwd1"] = "0"
	query["chyr1"] = "2024"
	query["wd1infl"] = "adj"
	query["signwd2"] = "+"
	query["chwd2"] = "0"
	query["chyr2"] = "2026"
	query["wd2infl"] = "adj"
	query["signwd3"] = "+"
	query["chwd3"] = "0"
	query["chyr3"] = "2030"
	query["wd3infl"] = "adj"
	query["holdyears"] = "2021"
	query["preadd"] = "0"
	query["inflpick"] = "4"
	query["override_inflation_rate"] = "3.0"
	query["SpendingModel"] = "constant"
	query["age"] = "48\t"
	query["pctlastyear"] = "0\t"
	query["infltype"] = "PPI"
	query["fixedinc"] = "Commercial+Paper"
	query["user_bonds"] = "4.0"
	query["InvExp"] = "0.18"
	query["monte"] = "history"
	query["StartYr"] = "1871"
	query["fixedchoice"] = "LongInterest"
	query["pctEquity"] = "75"
	query["mix1"] = "+10+++"
	query["mix2"] = "+10+++"
	query["mix3"] = "+10+++"
	query["mix4"] = "+40+++"
	query["mix5"] = "+40+++"
	query["mix6"] = "+10+++"
	query["mix7"] = "+15+++"
	query["mix8"] = "+5+++"
	query["user_inflation"] = "3.0"
	query["monte_growth"] = "10"
	query["monte_sd"] = "10"
	query["monte_inflation"] = "3.00"
	query["signlump1"] = "+"
	query["cashin1"] = "0"
	query["cashyr1"] = "2024"
	query["signlump2"] = "+"
	query["cashin2"] = "0"
	query["cashyr2"] = "2034"
	query["signlump3"] = "+"
	query["cashin3"] = "0"
	query["cashyr3"] = "2039"
	query["process"] = "survival"
	query["showyear"] = "1960\t\t"
	query["delay"] = "10"
	query["goal"] = "95"
	query["portfloor"] = "0"
	query["FIRECalcVersion"] = "3.0"

	// alt: url.Values{"key": {"Value"}, "id": {"123"}}
	params := url.Values{}
	for key, value := range query {
		params.Add(key, value)
	}

	//fmt.Printf("POST https://www.firecalc.com/firecalcresults.php\n")
	//fmt.Printf("PARAMS: %s\n", params.Encode())
	resp, err := http.PostForm("https://www.firecalc.com/firecalcresults.php", params)
	check(err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	check(err)
	//fmt.Println(string(body))

	r := regexp.MustCompile(`FIRECalc found that (\d+) cycles failed, for a success rate of\s+([\d\.]+)%`)
	m := r.FindStringSubmatch(string(body))
	//fmt.Printf("!!!!!!! m = %v\n", m)
	failedCycles, _ := strconv.Atoi(m[1])
	_ = failedCycles
	successRate, _ := strconv.ParseFloat(m[2], 64)
	fmt.Println(successRate)
	return successRate, nil
}
