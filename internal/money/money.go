package money

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var usdInput = regexp.MustCompile(`^(\d{1,3}(,\d{3})+|\d+)(\.\d{1,2})?$`)

func ParseUSD(text string) (int64, bool) {
	text = strings.TrimSpace(text)
	if !usdInput.MatchString(text) {
		return 0, false
	}
	whole, fraction, _ := strings.Cut(strings.ReplaceAll(text, ",", ""), ".")
	dollars, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || dollars > math.MaxInt64/100-1 {
		return 0, false
	}
	cents, err := strconv.ParseInt((fraction + "00")[:2], 10, 64)
	if err != nil {
		return 0, false
	}
	return dollars*100 + cents, true
}

func FormatUSD(cents int64) string {
	dollars := strconv.FormatInt(cents/100, 10)
	for i := len(dollars) - 3; i > 0; i -= 3 {
		dollars = dollars[:i] + "," + dollars[i:]
	}
	return fmt.Sprintf("$%s.%02d", dollars, cents%100)
}

func InputUSD(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
