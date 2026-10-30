package main

import "testing"

func TestRecalledFixtureAcceptsEquivalentDates(t *testing.T) {
	for _, date := range []string{"2030-05-16", "May 16, 2030", "16 May 2030"} {
		if !recalledFixture("Mira capybara-924 " + date) {
			t.Fatal("rejected equivalent date", date)
		}
	}
	for _, text := range []string{"Mira capybara-924 2030-05-17", "Mira capybara-923 May 16, 2030", "Noah capybara-924 2030-05-16"} {
		if recalledFixture(text) {
			t.Fatal("accepted wrong fact", text)
		}
	}
}
