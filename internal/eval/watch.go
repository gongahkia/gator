package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/jobs"
)

// WatchDataset is a temporal replay. Ground truth stays outside the watched
// workspace and is never included in the agent request.
type WatchDataset struct {
	Version   int            `json:"version"`
	ID        string         `json:"id"`
	Outcome   string         `json:"outcome"`
	AlertWhen string         `json:"alert_when"`
	Events    []WatchEvent   `json:"events"`
	Episodes  []WatchEpisode `json:"episodes"`
}

type WatchEvent struct {
	ID              string `json:"id"`
	Description     string `json:"description"`
	AvailableSecond int    `json:"available_second"`
	DeadlineSecond  int    `json:"deadline_second"`
}

type WatchEpisode struct {
	ID         string       `json:"id"`
	AtSecond   int          `json:"at_second"`
	SourceFile string       `json:"source_file"`
	Fault      string       `json:"fault,omitempty"` // Explicit synthetic fault, including in a live replay.
	Turns      []agent.Turn `json:"script,omitempty"`
}

func LoadWatchDataset(path string) (WatchDataset, error) {
	var d WatchDataset
	if err := readWatchJSON(path, &d); err != nil {
		return d, err
	}
	return d, d.Validate()
}

func (d WatchDataset) Validate() error {
	if d.Version != 1 || !identifierPattern.MatchString(d.ID) || strings.TrimSpace(d.Outcome) == "" || len(d.Outcome) > 4096 || strings.TrimSpace(d.AlertWhen) == "" || len(d.AlertWhen) > 2048 || len(d.Episodes) == 0 || len(d.Episodes) > 128 || len(d.Events) > 128 {
		return errors.New("invalid watch replay identity, agreement, or bounds")
	}
	seen := map[string]bool{}
	previous := -1
	for _, e := range d.Episodes {
		if !identifierPattern.MatchString(e.ID) || seen[e.ID] || e.AtSecond <= previous || e.AtSecond > 31*86400 || !filepath.IsLocal(e.SourceFile) || filepath.Clean(e.SourceFile) != e.SourceFile || strings.Contains(e.SourceFile, "\\") {
			return fmt.Errorf("invalid or unordered watch episode %q", e.ID)
		}
		if e.Fault != "" && e.Fault != "source" && e.Fault != "provider" && e.Fault != "tool" {
			return fmt.Errorf("unknown injected failure %q", e.Fault)
		}
		if len(e.Turns) > 32 {
			return errors.New("watch script exceeds 32 turns")
		}
		for _, turn := range e.Turns {
			if len(turn.Text) > 16384 || len(turn.ToolCalls) > 8 {
				return errors.New("watch script turn is too large")
			}
			for _, call := range turn.ToolCalls {
				// Fixture scripts inspect only the current source. No script can
				// supply a connector mutation, shell command, or arbitrary path.
				var arguments struct {
					Path string `json:"path"`
				}
				if call.Name != "read_file" || len(call.Arguments) > 4096 || json.Unmarshal(call.Arguments, &arguments) != nil || arguments.Path != "source/project.md" {
					return errors.New("watch fixture tools may only read source/project.md")
				}
			}
		}
		seen[e.ID], previous = true, e.AtSecond
	}
	seen = map[string]bool{}
	for _, e := range d.Events {
		if !identifierPattern.MatchString(e.ID) || seen[e.ID] || strings.TrimSpace(e.Description) == "" || len(e.Description) > 2048 || e.AvailableSecond < 0 || e.DeadlineSecond < e.AvailableSecond || e.DeadlineSecond > previous {
			return fmt.Errorf("invalid material event %q", e.ID)
		}
		opportunity := false
		for _, episode := range d.Episodes {
			if episode.AtSecond >= e.AvailableSecond && episode.AtSecond <= e.DeadlineSecond {
				opportunity = true
			}
		}
		if !opportunity {
			return fmt.Errorf("material event %q has no scheduled observation", e.ID)
		}
		seen[e.ID] = true
	}
	return nil
}

// ReadWatchSource confines fixtures to the dataset directory. Each version is
// read only when its episode starts; future versions are never copied in.
func ReadWatchSource(datasetPath string, episode WatchEpisode) ([]byte, error) {
	if !filepath.IsLocal(episode.SourceFile) {
		return nil, errors.New("invalid watch source")
	}
	root, err := os.OpenRoot(filepath.Dir(datasetPath))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(episode.SourceFile)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64*1024 {
		return nil, errors.New("watch source must be an ordinary file of at most 64 KiB")
	}
	file, err := root.Open(episode.SourceFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	if len(data) > 64*1024 {
		return nil, errors.New("watch source grew beyond 64 KiB")
	}
	return data, err
}

type WatchObservation struct {
	RecoveryInboxID string       `json:"recovery_inbox_id,omitempty"`
	EpisodeID       string       `json:"episode_id"`
	AtSecond        int          `json:"at_second"`
	SourceSHA256    string       `json:"source_sha256"`
	Attempt         jobs.Attempt `json:"attempt"`
	InboxID         string       `json:"inbox_id,omitempty"`
	Alert           bool         `json:"alert"` // Completed, surfaced change/review; excludes health notices.
	HealthNotice    bool         `json:"health_notice"`
	FailureCategory string       `json:"failure_category,omitempty"`
	FaultInjected   string       `json:"fault_injected,omitempty"`
	ProviderErrors  int          `json:"provider_errors"`
	ToolErrors      int          `json:"tool_errors"`
	ModelCalls      int          `json:"model_calls"`
	OutputSHA256    string       `json:"output_sha256"`
}

func (o WatchObservation) OutputDigest() string {
	payload, _ := json.Marshal(struct {
		Attempt       jobs.Attempt
		Recovery      string
		Inbox         string
		Alert, Health bool
	}{o.Attempt, o.RecoveryInboxID, o.InboxID, o.Alert, o.HealthNotice})
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

type WatchReport struct {
	Version      int                `json:"version"`
	ID           string             `json:"id"`
	Dataset      WatchDataset       `json:"dataset"`
	Mode         string             `json:"mode"`
	Provider     string             `json:"provider,omitempty"`
	Model        string             `json:"model,omitempty"`
	Harness      string             `json:"harness"`
	StartedAt    time.Time          `json:"started_at"`
	Complete     bool               `json:"complete"`
	StopReason   string             `json:"stop_reason,omitempty"`
	Observations []WatchObservation `json:"observations"`
}

type WatchReviews struct {
	Version  int           `json:"version"`
	ReportID string        `json:"report_id"`
	Evidence string        `json:"evidence"` // human or scripted_fixture; never inferred.
	Reviewer string        `json:"reviewer"`
	Items    []WatchReview `json:"items"`
}

type WatchReview struct {
	EpisodeID        string   `json:"episode_id"`
	AttemptID        string   `json:"attempt_id"`
	OutputSHA256     string   `json:"output_sha256"`
	ReportedEventIDs []string `json:"reported_event_ids"`
	Useful           *bool    `json:"useful,omitempty"`
	Actionable       *bool    `json:"actionable,omitempty"`
	Note             string   `json:"note"`
}

type WatchRate struct {
	Numerator   int      `json:"numerator"`
	Denominator int      `json:"denominator"`
	Value       *float64 `json:"value"` // null when unmeasured, never a fabricated zero.
}

func watchRate(n, d int) WatchRate {
	r := WatchRate{Numerator: n, Denominator: d}
	if d > 0 {
		value := float64(n) / float64(d)
		r.Value = &value
	}
	return r
}

type WatchScore struct {
	Evidence                 string         `json:"evidence"`
	ExecutionMode            string         `json:"execution_mode"`
	Episodes                 int            `json:"episodes"`
	ExpectedEpisodes         int            `json:"expected_episodes"`
	ReplayComplete           bool           `json:"replay_complete"`
	Completed                int            `json:"completed"`
	Unchanged                int            `json:"unchanged"`
	RecoveryNotices          int            `json:"recovery_notices"`
	HealthNotices            int            `json:"health_notices"`
	Alerts                   int            `json:"alerts"`
	ReviewedAlerts           int            `json:"reviewed_alerts"`
	UnreviewedAlerts         int            `json:"unreviewed_alerts"`
	Failures                 map[string]int `json:"failures"`
	ProviderErrors           int            `json:"provider_errors"`
	ToolErrors               int            `json:"tool_errors"`
	ModelCalls               int            `json:"model_calls"`
	UsefulAlertPrecision     WatchRate      `json:"useful_alert_precision"`
	Actionability            WatchRate      `json:"actionability"`
	MissedMaterialChanges    WatchRate      `json:"missed_material_changes"`
	RepeatedAlerts           WatchRate      `json:"repeated_alerts"`
	DetectionDelaySeconds    WatchRate      `json:"mean_detection_delay_seconds"`
	UnassessedMaterialEvents []string       `json:"unassessed_material_events"`
	UnassessedDelayEventIDs  []string       `json:"unassessed_delay_event_ids"`
	UnassessedRepeatAlerts   int            `json:"unassessed_repeat_alerts"`
	MissedEventIDs           []string       `json:"missed_event_ids"`
	DetectedEventIDs         []string       `json:"detected_event_ids"`
}

func ScoreWatch(report WatchReport, reviews WatchReviews) (WatchScore, error) {
	score := WatchScore{Evidence: "unreviewed", ExecutionMode: report.Mode, Failures: map[string]int{}}
	if err := report.Dataset.Validate(); err != nil {
		return score, err
	}
	if report.Version != 1 || report.ID == "" || (report.Mode != "scripted" && report.Mode != "live") || len(report.Observations) > len(report.Dataset.Episodes) || report.Complete && len(report.Observations) != len(report.Dataset.Episodes) {
		return score, errors.New("invalid watch report")
	}
	score.ExpectedEpisodes, score.ReplayComplete = len(report.Dataset.Episodes), report.Complete
	events := map[string]WatchEvent{}
	for _, event := range report.Dataset.Events {
		events[event.ID] = event
	}
	byEpisode := map[string]WatchObservation{}
	for index, o := range report.Observations {
		episode := report.Dataset.Episodes[index]
		if o.EpisodeID != episode.ID || o.AtSecond != episode.AtSecond || o.Attempt.ID == "" || o.OutputSHA256 != o.OutputDigest() {
			return score, errors.New("watch observation identity or digest mismatch")
		}
		if o.Alert != (o.InboxID != "" && o.Attempt.Status == "completed") || o.HealthNotice != (o.InboxID != "" && o.Attempt.Status != "completed") || o.ModelCalls < 0 || o.ProviderErrors < 0 || o.ToolErrors < 0 || (o.Attempt.Status == "completed" || o.Attempt.Status == "unchanged") && o.FailureCategory != "" {
			return score, errors.New("watch observation has inconsistent execution or delivery state")
		}
		if o.RecoveryInboxID != "" && (o.Attempt.Status != "completed" && o.Attempt.Status != "unchanged" || o.RecoveryInboxID == o.InboxID) {
			return score, errors.New("inconsistent recovery notice")
		}
		byEpisode[o.EpisodeID] = o
	}
	labels := map[string]WatchReview{}
	if reviews.Version == 0 && (len(reviews.Items) != 0 || reviews.ReportID != "" || reviews.Evidence != "" || reviews.Reviewer != "") {
		return score, errors.New("watch review version is required")
	}
	if reviews.Version != 0 {
		if reviews.Version != 1 || reviews.ReportID != report.ID || (reviews.Evidence != "human" && reviews.Evidence != "scripted_fixture") || strings.TrimSpace(reviews.Reviewer) == "" {
			return score, errors.New("watch reviews must identify the report, reviewer and evidence kind")
		}
		if reviews.Evidence == "scripted_fixture" && report.Mode != "scripted" {
			return score, errors.New("fixture reviews cannot grade live outputs")
		}
		score.Evidence = reviews.Evidence
		for _, review := range reviews.Items {
			o, exists := byEpisode[review.EpisodeID]
			_, duplicate := labels[review.EpisodeID]
			if !exists || duplicate || !o.Alert || o.FailureCategory != "" || review.AttemptID != o.Attempt.ID || review.OutputSHA256 != o.OutputSHA256 || len(review.Note) > 4096 || strings.TrimSpace(review.Note) == "" {
				return score, errors.New("review must bind one surfaced alert and include a source-supported note")
			}
			seen := map[string]bool{}
			for _, id := range review.ReportedEventIDs {
				e, exists := events[id]
				if !exists || seen[id] || e.AvailableSecond > o.AtSecond {
					return score, errors.New("review contains an unknown, duplicate or future material event")
				}
				seen[id] = true
			}
			labels[review.EpisodeID] = review
		}
	}
	first := map[string]int{}
	useful, ratedUseful, actionable, ratedActionable, repeats, repeatAssessed := 0, 0, 0, 0, 0, 0
	earlierUnreviewed := func(eventID string, before int) bool {
		for _, prior := range report.Observations {
			if prior.AtSecond >= before {
				break
			}
			if prior.Alert && prior.AtSecond >= events[eventID].AvailableSecond {
				if _, ok := labels[prior.EpisodeID]; !ok {
					return true
				}
			}
		}
		return false
	}
	for _, o := range report.Observations {
		score.Episodes++
		if o.RecoveryInboxID != "" {
			score.RecoveryNotices++
		}
		score.ProviderErrors += o.ProviderErrors
		score.ToolErrors += o.ToolErrors
		score.ModelCalls += o.ModelCalls
		if o.Attempt.Status == "completed" {
			score.Completed++
		}
		if o.Attempt.Status == "unchanged" {
			score.Unchanged++
		}
		if o.HealthNotice {
			score.HealthNotices++
		}
		if o.FailureCategory != "" {
			score.Failures[o.FailureCategory]++
			continue
		}
		if !o.Alert {
			continue
		}
		score.Alerts++
		r, reviewed := labels[o.EpisodeID]
		if !reviewed {
			score.UnreviewedAlerts++
			continue
		}
		score.ReviewedAlerts++
		if r.Useful != nil {
			ratedUseful++
			if *r.Useful {
				useful++
			}
		}
		if r.Actionable != nil {
			ratedActionable++
			if *r.Actionable {
				actionable++
			}
		}
		repeated, uncertainRepeat := false, false
		for _, id := range r.ReportedEventIDs {
			if _, exists := first[id]; exists {
				repeated = true
			} else {
				first[id] = o.AtSecond
				uncertainRepeat = uncertainRepeat || earlierUnreviewed(id, o.AtSecond)
			}
		}
		if repeated {
			repeats++
		}
		if repeated || !uncertainRepeat {
			repeatAssessed++
		} else {
			score.UnassessedRepeatAlerts++
		}
	}
	score.UsefulAlertPrecision, score.Actionability = watchRate(useful, ratedUseful), watchRate(actionable, ratedActionable)
	score.RepeatedAlerts = watchRate(repeats, repeatAssessed)
	assessed, missed, delay, detected := 0, 0, 0, 0
	for _, event := range report.Dataset.Events {
		when, found := first[event.ID]
		if found {
			score.DetectedEventIDs = append(score.DetectedEventIDs, event.ID)
			if earlierUnreviewed(event.ID, when) {
				score.UnassessedDelayEventIDs = append(score.UnassessedDelayEventIDs, event.ID)
			} else {
				delay += when - event.AvailableSecond
				detected++
			}
		}
		healthy, unreviewed := false, false
		for _, o := range report.Observations {
			if o.AtSecond < event.AvailableSecond || o.AtSecond > event.DeadlineSecond {
				continue
			}
			if o.FailureCategory == "" && (o.Attempt.Status == "completed" || o.Attempt.Status == "unchanged") {
				healthy = true
			}
			if o.Alert {
				if _, ok := labels[o.EpisodeID]; !ok {
					unreviewed = true
				}
			}
		}
		if found && when <= event.DeadlineSecond {
			assessed++
			continue
		}
		windowFinished := report.Complete || len(report.Observations) > 0 && report.Observations[len(report.Observations)-1].AtSecond >= event.DeadlineSecond
		if !healthy || unreviewed || !windowFinished {
			score.UnassessedMaterialEvents = append(score.UnassessedMaterialEvents, event.ID)
			continue
		}
		assessed++
		missed++
		score.MissedEventIDs = append(score.MissedEventIDs, event.ID)
	}
	score.MissedMaterialChanges, score.DetectionDelaySeconds = watchRate(missed, assessed), watchRate(delay, detected)
	return score, nil
}

func SaveWatchReport(path string, report WatchReport) error {
	return writeJSON(path, report, "watch report")
}
func LoadWatchReport(path string) (WatchReport, error) {
	var r WatchReport
	err := readWatchJSON(path, &r)
	return r, err
}
func LoadWatchReviews(path string) (WatchReviews, error) {
	var r WatchReviews
	err := readWatchJSON(path, &r)
	return r, err
}

func readWatchJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 16*1024*1024 {
		return errors.New("watch JSON must be an ordinary file of at most 16 MiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 16*1024*1024+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("watch JSON has trailing data or exceeds its bound")
	}
	return nil
}
