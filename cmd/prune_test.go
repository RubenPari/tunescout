package cmd

import (
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	for in, want := range map[string]bool{
		"s\n": true, "Sì\n": true, " yes \n": true, "y": true,
		"\n": false, "n\n": false, "no\n": false, "": false, "forse\n": false,
	} {
		got, err := confirm(strings.NewReader(in), "")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}
