package main

import "testing"

func TestParseReviewWebOptionsAcceptsDocumentedPathFirstAndFlagFirstForms(t *testing.T) {
	for _, arguments := range [][]string{
		{"/tmp/run", "--listen", "127.0.0.1:39001", "--open"},
		{"--open", "--listen=127.0.0.1:39001", "/tmp/run"},
	} {
		options, err := parseReviewWebOptions(arguments)
		if err != nil || options.statePath != "/tmp/run" || options.listen != "127.0.0.1:39001" || !options.open {
			t.Fatalf("parse %q = %#v, %v", arguments, options, err)
		}
	}
}

func TestParseReviewWebOptionsRejectsRemoteListener(t *testing.T) {
	if _, err := parseReviewWebOptions([]string{"/tmp/run", "--listen", "0.0.0.0:39001"}); err == nil {
		t.Fatal("remote review listener accepted")
	}
}
