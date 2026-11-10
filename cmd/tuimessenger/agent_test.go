package main

import (
	"flag"
	"io"
	"slices"
	"testing"
)

func TestParseAnywhere(t *testing.T) {
	for _, tc := range []struct {
		args []string
		pos  []string
		yes  bool
		lim  int
	}{
		{[]string{"maya", "on my way", "-yes"}, []string{"maya", "on my way"}, true, 0},
		{[]string{"-yes", "maya", "hi"}, []string{"maya", "hi"}, true, 0},
		{[]string{"-limit", "5", "maya"}, []string{"maya"}, false, 5},
		{[]string{"maya", "-limit=7"}, []string{"maya"}, false, 7},
		{[]string{"maya", "--", "-yes is text here"}, []string{"maya", "-yes is text here"}, false, 0},
		{[]string{"-yes", "--", "maya", "-limit"}, []string{"maya", "-limit"}, true, 0},
	} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		yes := fs.Bool("yes", false, "")
		lim := fs.Int("limit", 0, "")
		pos, err := parseAnywhere(fs, tc.args)
		if err != nil || !slices.Equal(pos, tc.pos) || *yes != tc.yes || *lim != tc.lim {
			t.Errorf("%q: pos=%q yes=%v limit=%d err=%v", tc.args, pos, *yes, *lim, err)
		}
	}
}
