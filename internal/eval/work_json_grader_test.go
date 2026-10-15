package eval

import "testing"

func TestJSONPointerGraderReadsTypedValuesAndAbsence(t *testing.T) {
	data := []byte(`{"totals":{"first_month":"1800.00","approved":true,"unknown":null},"a/b":{"~key":["ready"]}}`)
	for _, tc := range []struct {
		pointer  string
		expected any
		found    bool
	}{
		{"/totals/first_month", "1800.00", true},
		{"/totals/approved", true, true},
		{"/totals/unknown", nil, true},
		{"/totals/missing", nil, false},
		{"/a~1b/~0key/0", "ready", true},
		{"/a~1b/~0key/01", nil, false},
	} {
		got, found, err := jsonPointerValue(data, tc.pointer)
		if err != nil || found != tc.found || !jsonValuesEqual(got, tc.expected) {
			t.Errorf("pointer %q = %#v, %t, %v", tc.pointer, got, found, err)
		}
	}
}

func TestJSONPointerGraderRejectsMalformedCriteriaAndOutput(t *testing.T) {
	for _, grader := range []WorkGrader{
		{Kind: "json_pointer_equals", Path: "answer.json", Locator: "/a~2b", Expected: `"x"`},
		{Kind: "json_pointer_equals", Path: "answer.json", Locator: "/x", Expected: `"x" "y"`},
		{Kind: "json_pointer_absent", Path: "answer.json", Locator: "/x", Expected: `null`},
		{Kind: "json_pointer_contains", Path: "answer.json", Locator: "/x"},
		{Kind: "file_excludes", Path: "answer.json"},
	} {
		if err := validateStructuredGrader(grader); err == nil {
			t.Errorf("accepted invalid grader %#v", grader)
		}
	}
	if _, _, err := jsonPointerValue([]byte(`{"x":1} {"y":2}`), "/x"); err == nil {
		t.Fatal("accepted multiple output JSON values")
	}
	if value, found, err := jsonPointerValue([]byte(`{"x":null}`), "/x"); err != nil || !found || value != nil {
		t.Fatalf("present null was treated as absent: %#v, %t, %v", value, found, err)
	}
}
