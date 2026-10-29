package memory

import (
	"errors"
	"fmt"
)

type ReadIssue struct {
	File string `json:"file"`
}

// ReadIssues accompanies a partial, validated result. Read-only consumers may
// use those records only while surfacing the warning. Mutation paths must keep
// rejecting it: an unreadable record could contain a conflicting subject.
type ReadIssues struct {
	Count int         `json:"count"`
	Items []ReadIssue `json:"items"`
}

func (issues *ReadIssues) Error() string {
	return fmt.Sprintf("%d saved memory record(s) unreadable or invalid; healthy records remain available. Original files are retained. Run gator memory diagnose to inspect.", issues.Count)
}

func (issues *ReadIssues) add(name string) {
	issues.Count++
	if len(issues.Items) < 32 {
		issues.Items = append(issues.Items, ReadIssue{File: name})
	}
}

// ReadWarning returns an explicitly recoverable partial-read notice. An empty
// string does not turn other errors (including inaccessible stores) into success.
func ReadWarning(err error) string {
	var issues *ReadIssues
	if errors.As(err, &issues) {
		return issues.Error()
	}
	return ""
}
