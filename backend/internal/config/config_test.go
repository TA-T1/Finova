package config

import (
	"reflect"
	"testing"
)

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" http://localhost:3000, ,https://app.example ")
	want := []string{"http://localhost:3000", "https://app.example"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitCSV() = %#v, want %#v", got, want)
	}
}
