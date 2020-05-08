package main

import "testing"

func TestInvalidHTML(t *testing.T) {
	for _, input := range []string{
		"<p>1 < 5</p>",
		"<p>1 <> 5</p>",
	} {
		if err := IsValidHTML(input); err != nil {
			t.Logf("%s: %v", input, err)
		} else {
			t.Errorf("%s: this is invalid but no error returned!", input)
		}
	}
}
