package datasource

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // make Asia/Tehran available even on minimal images

	ptime "github.com/yaa110/go-persian-calendar"
)

// Tehran is the market time zone. Iran has no DST since 2022, so it is a fixed
// +03:30 offset; the FixedZone fallback matches that if tzdata is unavailable.
var Tehran = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Tehran"); err == nil {
		return loc
	}
	return time.FixedZone("Asia/Tehran", 3*3600+1800)
}()

var textReplacer = strings.NewReplacer(
	// Arabic letters -> Persian.
	"ي", "ی", "ى", "ی", "ك", "ک",
	// Zero-width / directional marks: ZWNJ, ZWSP, ZWJ, LRM, RLM, BOM, word joiner.
	"\u200c", "", "\u200b", "", "\u200d", "", "\u200e", "", "\u200f", "", "\ufeff", "", "\u2060", "",
	// Persian digits.
	"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4", "۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9",
	// Arabic-Indic digits.
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4", "٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
	// Arabic decimal / thousands separators.
	"٫", ".", "٬", ",",
)

// NormalizeText converts Arabic ي/ك to Persian ی/ک, converts Persian and Arabic
// digits to ASCII, removes zero-width characters and trims whitespace. Use it
// for every symbol, name and numeric string coming from a source.
func NormalizeText(s string) string {
	return strings.TrimSpace(textReplacer.Replace(s))
}

// JalaliToGregorian converts a Jalali (Persian calendar) date to a Gregorian
// date at 00:00 in Asia/Tehran. It rejects impossible dates.
func JalaliToGregorian(year, month, day int) (time.Time, error) {
	if month < 1 || month > 12 || day < 1 || day > 31 || year < 1000 || year > 1999 {
		return time.Time{}, fmt.Errorf("invalid jalali date %d/%d/%d", year, month, day)
	}
	p := ptime.Date(year, ptime.Month(month), day, 0, 0, 0, 0, Tehran)
	g := p.Time()
	// ptime normalizes overflow (e.g. 1405/12/31 in a non-leap year); detect it.
	back := ptime.New(g)
	if back.Year() != year || int(back.Month()) != month || back.Day() != day {
		return time.Time{}, fmt.Errorf("invalid jalali date %d/%d/%d", year, month, day)
	}
	return time.Date(g.Year(), g.Month(), g.Day(), 0, 0, 0, 0, Tehran), nil
}

// ParseJalali parses "1405/08/27" (also with "-" separators, and with Persian
// or Arabic digits) into a Gregorian date at 00:00 Asia/Tehran.
func ParseJalali(s string) (time.Time, error) {
	n := strings.NewReplacer("-", "/", ".", "/").Replace(NormalizeText(s))
	var y, m, d int
	if _, err := fmt.Sscanf(n, "%d/%d/%d", &y, &m, &d); err != nil {
		return time.Time{}, fmt.Errorf("parse jalali date %q: %w", s, err)
	}
	return JalaliToGregorian(y, m, d)
}

// ParseGregorianYMD parses a Gregorian "YYYYMMDD" string into a date at 00:00
// Asia/Tehran.
func ParseGregorianYMD(s string) (time.Time, error) {
	s = NormalizeText(s)
	if len(s) != 8 {
		return time.Time{}, fmt.Errorf("parse date %q: want YYYYMMDD", s)
	}
	var y, m, d int
	if _, err := fmt.Sscanf(s, "%4d%2d%2d", &y, &m, &d); err != nil {
		return time.Time{}, fmt.Errorf("parse date %q: %w", s, err)
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, Tehran)
	if t.Year() != y || int(t.Month()) != m || t.Day() != d || y < 1900 {
		return time.Time{}, fmt.Errorf("parse date %q: not a valid date", s)
	}
	return t, nil
}
