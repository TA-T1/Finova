package datasource

import (
	"testing"
	"time"
)

func TestNormalizeText(t *testing.T) {
	cases := map[string]string{
		"فملي":               "فملی",       // Arabic ي
		"كاشي":               "کاشی",       // Arabic ك + ي
		"  ضخود\u200c1234  ": "ضخود1234",   // ZWNJ + spaces
		"\ufeffخودرو\u200f":  "خودرو",      // BOM + RLM
		"۱۲۳۴۵۶۷۸۹۰":         "1234567890", // Persian digits
		"٠١٢٣٤٥٦٧٨٩":         "0123456789", // Arabic-Indic digits
		"ضهرم۸۰۳۰":           "ضهرم8030",   // mixed
		"":                   "",
	}
	for in, want := range cases {
		if got := NormalizeText(in); got != want {
			t.Errorf("NormalizeText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseJalali(t *testing.T) {
	ok := map[string]time.Time{
		"1405/08/27": time.Date(2026, 11, 18, 0, 0, 0, 0, Tehran),
		"1403/01/01": time.Date(2024, 3, 20, 0, 0, 0, 0, Tehran),
		"۱۴۰۵/۰۸/۲۷": time.Date(2026, 11, 18, 0, 0, 0, 0, Tehran), // Persian digits
		"1405-08-27": time.Date(2026, 11, 18, 0, 0, 0, 0, Tehran),
		"1403/12/30": time.Date(2025, 3, 20, 0, 0, 0, 0, Tehran), // leap year
	}
	for in, want := range ok {
		got, err := ParseJalali(in)
		if err != nil || !got.Equal(want) {
			t.Errorf("ParseJalali(%q) = %v, %v; want %v", in, got, err, want)
		}
		if err == nil && got.Location().String() != "Asia/Tehran" {
			t.Errorf("ParseJalali(%q) location = %v", in, got.Location())
		}
	}
	for _, bad := range []string{"", "abc", "1405/13/01", "1405/00/10", "1405/12/30", "1405/07/32", "1405/08"} {
		if got, err := ParseJalali(bad); err == nil {
			t.Errorf("ParseJalali(%q) = %v, want error", bad, got)
		}
	}
}

func TestParseGregorianYMD(t *testing.T) {
	got, err := ParseGregorianYMD("20261118")
	if want := time.Date(2026, 11, 18, 0, 0, 0, 0, Tehran); err != nil || !got.Equal(want) {
		t.Errorf("got %v, %v; want %v", got, err, want)
	}
	for _, bad := range []string{"", "2026118", "20261340", "20260230", "abcdefgh"} {
		if _, err := ParseGregorianYMD(bad); err == nil {
			t.Errorf("ParseGregorianYMD(%q): want error", bad)
		}
	}
}
