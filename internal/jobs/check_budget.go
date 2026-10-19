package jobs

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

type checkReservation struct {
	Day string `json:"day"`
}

// ReserveCheck atomically reserves one Work check across supervisor and manual
// runs. Source-only unchanged polls do not reserve a check. Failures after a
// reservation still consume the slot, keeping the bound conservative.
func (s Store) ReserveCheck(definition Definition, attempt Attempt, now time.Time) (bool, error) {
	if definition.MaxChecksPerDay == 0 {
		return true, nil
	}
	if invalidID(definition.ID) || attempt.JobID != definition.ID || invalidID(attempt.ID) {
		return false, errors.New("invalid daily check reservation")
	}
	unlock, err := s.lockDefinitions()
	if err != nil {
		return false, err
	}
	defer unlock()
	path := filepath.Join(s.root, "history", definition.ID, attempt.ID, "check-budget.json")
	var prior checkReservation
	if err := readJSON(path, &prior); err == nil {
		return prior.Day == now.UTC().Format("2006-01-02"), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	count, err := s.DailyChecks(definition.ID, now)
	if err != nil {
		return false, err
	}
	if count >= definition.MaxChecksPerDay {
		return false, nil
	}
	if err := writeJSONExclusive(path, checkReservation{Day: now.UTC().Format("2006-01-02")}); err != nil {
		return false, err
	}
	return true, nil
}

func (s Store) DailyChecks(jobID string, now time.Time) (int, error) {
	history, err := s.History(jobID, 0)
	if err != nil {
		return 0, err
	}
	count := 0
	day := now.UTC().Format("2006-01-02")
	for _, attempt := range history {
		var reservation checkReservation
		path := filepath.Join(s.root, "history", jobID, attempt.ID, "check-budget.json")
		if err := readJSON(path, &reservation); err == nil {
			if reservation.Day == day {
				count++
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		if len(attempt.Runs) > 0 && attempt.StartedAt.UTC().Format("2006-01-02") == day {
			count++
		}
	}
	return count, nil
}
