package workapp

import (
	"fmt"
	"regexp"

	tea "github.com/charmbracelet/bubbletea"
)

type BridgeCheckAction struct {
	RequestID string `json:"request_id"`
	ID        string `json:"id,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Action    string `json:"action"`
}

type BridgeCheckResult struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

type bridgeCheckDone struct {
	Action    BridgeCheckAction
	Refreshed checksRefreshed
	Err       error
}

var bridgeCheckIdentity = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$`)
var bridgeCheckRequest = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)
var bridgeCheckDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Card controls use the same check store/forms as the existing application.
// No renderer fields can change an agreement or select unchecked execution.
func (m Model) ApplyBridgeCheck(action BridgeCheckAction) (tea.Model, tea.Cmd) {
	m.bridgeCheckResult = nil
	fail := func(text string) (tea.Model, tea.Cmd) {
		m.bridgeCheckResult = &BridgeCheckResult{RequestID: action.RequestID, Error: text}
		return m, nil
	}
	if !bridgeCheckRequest.MatchString(action.RequestID) || m.config.ReadChecks == nil {
		return fail("Check controls are unavailable.")
	}
	if action.Action != "refresh" && action.Action != "inspect" && action.Action != "edit" && action.Action != "pause" && action.Action != "resume" {
		return fail("Invalid check action.")
	}
	if action.Action == "refresh" && (action.ID != "" || action.Digest != "") || action.Action != "refresh" && (!bridgeCheckIdentity.MatchString(action.ID) || !bridgeCheckDigest.MatchString(action.Digest)) {
		return fail("Review the current check before changing it.")
	}
	if m.running || m.followupSaving || m.controlBusy || m.bridgeExternal != nil || m.followupForm != nil {
		return fail("Finish the current operation before managing this check.")
	}
	m.followupSaving = true
	read, setEnabled := m.config.ReadChecks, m.config.SetCheckEnabled
	return m, func() tea.Msg {
		checks, online, err := read()
		result := bridgeCheckDone{Action: action, Refreshed: checksRefreshed{Checks: checks, Running: online, Err: err}, Err: err}
		if err != nil || action.Action == "refresh" {
			return result
		}
		for _, check := range checks {
			if check.ID != action.ID {
				continue
			}
			digest, digestErr := check.Definition.PolicyDigest()
			if digestErr != nil || digest != action.Digest {
				result.Err = fmt.Errorf("this check changed; refresh and review it again")
				return result
			}
			if action.Action == "edit" && !check.Definition.GoalWatch {
				result.Err = fmt.Errorf("this schedule uses advanced settings; open /jobs to edit it")
				return result
			}
			if action.Action == "pause" || action.Action == "resume" {
				if setEnabled == nil {
					result.Err = fmt.Errorf("check changes are unavailable")
					return result
				}
				if action.Action == "resume" && check.State == "goal paused" {
					result.Err = fmt.Errorf("resume the goal before its recurring check")
					return result
				}
				result.Err = setEnabled(check.ID, action.Action == "resume", action.Digest)
				if result.Err == nil {
					result.Refreshed.Checks, result.Refreshed.Running, result.Refreshed.Err = read()
					result.Err = result.Refreshed.Err
					if result.Err != nil {
						verb := "paused"
						if action.Action == "resume" {
							verb = "enabled"
						}
						result.Err = fmt.Errorf("check %s, but refreshed health is unavailable: %w", verb, result.Err)
					}
				}
			}
			return result
		}
		result.Err = fmt.Errorf("this check no longer exists; refresh Following")
		return result
	}
}

func (m Model) finishBridgeCheck(value bridgeCheckDone) (tea.Model, tea.Cmd) {
	m.followupSaving = false
	next, _ := m.Update(value.Refreshed)
	m = next.(Model)
	m.bridgeCheckResult = &BridgeCheckResult{RequestID: value.Action.RequestID}
	if value.Err != nil {
		m.bridgeCheckResult.Error = value.Err.Error()
		return m, nil
	}
	if value.Action.Action == "inspect" || value.Action.Action == "edit" {
		for _, check := range m.checks {
			if check.ID != value.Action.ID {
				continue
			}
			copy := check
			m.section, m.checkDetail, m.selected, m.launcher, m.home = "followup", &copy, 0, false, false
			if value.Action.Action == "edit" {
				m.editFollowup()
				if m.followupForm == nil {
					m.bridgeCheckResult.Error = m.sectionNotice
				}
			}
			break
		}
	}
	return m, nil
}
