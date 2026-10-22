package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/gator/internal/jobs"
)

func watchBool(v bool) *bool { return &v }

func scoreFixture() (WatchReport, WatchReviews) {
	d := WatchDataset{Version: 1, ID: "temporal", Outcome: "Watch commitments", AlertWhen: "A material change", Events: []WatchEvent{
		{ID: "deadline", Description: "Deadline changed", AvailableSecond: 10, DeadlineSecond: 30},
		{ID: "owner", Description: "Owner changed", AvailableSecond: 40, DeadlineSecond: 50},
		{ID: "blocked", Description: "Could not inspect", AvailableSecond: 60, DeadlineSecond: 60},
	}, Episodes: []WatchEpisode{{ID: "first", AtSecond: 10, SourceFile: "first.md"}, {ID: "repeat", AtSecond: 20, SourceFile: "repeat.md"}, {ID: "noise", AtSecond: 30, SourceFile: "noise.md"}, {ID: "miss", AtSecond: 50, SourceFile: "miss.md"}, {ID: "failure", AtSecond: 60, SourceFile: "failure.md"}}}
	r := WatchReport{Version: 1, ID: "report", Mode: "scripted", Dataset: d, Complete: true}
	reviews := WatchReviews{Version: 1, ReportID: r.ID, Evidence: "scripted_fixture", Reviewer: "test"}
	for _, episode := range d.Episodes {
		o := WatchObservation{EpisodeID: episode.ID, AtSecond: episode.AtSecond, Attempt: jobs.Attempt{ID: "attempt-" + episode.ID, Status: "completed", Summary: episode.ID}}
		if episode.ID == "failure" {
			o.Attempt.Status, o.FailureCategory, o.HealthNotice, o.InboxID = "failed", "provider", true, "health"
		} else if episode.ID != "miss" {
			o.Alert, o.InboxID = true, "inbox-"+episode.ID
		}
		o.OutputSHA256 = o.OutputDigest()
		r.Observations = append(r.Observations, o)
		if o.Alert {
			label := WatchReview{EpisodeID: o.EpisodeID, AttemptID: o.Attempt.ID, OutputSHA256: o.OutputSHA256, Useful: watchBool(episode.ID == "first"), Actionable: watchBool(episode.ID != "noise"), Note: "Synthetic judgment against fixture truth."}
			if episode.ID != "noise" {
				label.ReportedEventIDs = []string{"deadline"}
			}
			reviews.Items = append(reviews.Items, label)
		}
	}
	return r, reviews
}

func TestWatchScoreUsesExplicitDenominatorsAndExcludesFailures(t *testing.T) {
	report, reviews := scoreFixture()
	score, err := ScoreWatch(report, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if score.UsefulAlertPrecision.Numerator != 1 || score.UsefulAlertPrecision.Denominator != 3 || score.Actionability.Numerator != 2 || score.Actionability.Denominator != 3 || score.RepeatedAlerts.Numerator != 1 || score.RepeatedAlerts.Denominator != 3 {
		t.Fatalf("incorrect alert metrics: %#v", score)
	}
	if score.MissedMaterialChanges.Numerator != 1 || score.MissedMaterialChanges.Denominator != 2 || len(score.UnassessedMaterialEvents) != 1 || score.UnassessedMaterialEvents[0] != "blocked" || score.Failures["provider"] != 1 || score.HealthNotices != 1 {
		t.Fatalf("failure was scored as a quality miss: %#v", score)
	}
	if score.DetectionDelaySeconds.Denominator != 1 || score.DetectionDelaySeconds.Value == nil || *score.DetectionDelaySeconds.Value != 0 {
		t.Fatalf("incorrect event delay: %#v", score.DetectionDelaySeconds)
	}
}

func TestWatchScoreLeavesUnreviewedAlertsAndPartialWindowsUnscored(t *testing.T) {
	report, _ := scoreFixture()
	score, err := ScoreWatch(report, WatchReviews{})
	if err != nil {
		t.Fatal(err)
	}
	if score.UsefulAlertPrecision.Value != nil || score.RepeatedAlerts.Value != nil || score.UnreviewedAlerts != 3 || len(score.UnassessedMaterialEvents) != 2 {
		t.Fatalf("unreviewed outcomes fabricated quality: %#v", score)
	}
	report.Complete = false
	report.Observations = report.Observations[:1]
	score, err = ScoreWatch(report, WatchReviews{})
	if err != nil || score.MissedMaterialChanges.Denominator != 0 || len(score.UnassessedMaterialEvents) != 3 {
		t.Fatalf("partial replay counted future misses: %#v %v", score, err)
	}
}

func TestWatchScoreBindsReviewsAndCountsLateDetection(t *testing.T) {
	report, reviews := scoreFixture()
	reviews.Items[0].AttemptID = "other"
	if _, err := ScoreWatch(report, reviews); err == nil {
		t.Fatal("accepted review of a different attempt")
	}
	report, reviews = scoreFixture()
	reviews.Items[0].ReportedEventIDs = []string{"owner"}
	if _, err := ScoreWatch(report, reviews); err == nil {
		t.Fatal("accepted a future event")
	}
	report, reviews = scoreFixture()
	report.Mode = "live"
	if _, err := ScoreWatch(report, reviews); err == nil {
		t.Fatal("fixture labels graded a live run")
	}
	report, reviews = scoreFixture()
	report.Dataset.Events[0].DeadlineSecond = 10
	reviews.Items[0].ReportedEventIDs = nil
	score, err := ScoreWatch(report, reviews)
	if err != nil || score.MissedMaterialChanges.Numerator != 2 || score.DetectionDelaySeconds.Numerator != 10 {
		t.Fatalf("late notification hid a missed deadline: %#v %v", score, err)
	}
}

func TestWatchScoreDoesNotInventDelayOrRepetitionFromPartialReviews(t *testing.T) {
	report, reviews := scoreFixture()
	reviews.Items = reviews.Items[1:]
	score, err := ScoreWatch(report, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if score.DetectionDelaySeconds.Value != nil || len(score.UnassessedDelayEventIDs) != 1 || score.UnassessedDelayEventIDs[0] != "deadline" || score.UnassessedRepeatAlerts != 1 || score.RepeatedAlerts.Denominator != 1 {
		t.Fatalf("unreviewed earlier alert fabricated first detection/repeat status: %#v", score)
	}
	// The reviewed second alert still proves this event was delivered on time.
	if score.MissedMaterialChanges.Numerator != 1 || score.MissedMaterialChanges.Denominator != 2 {
		t.Fatalf("known on-time delivery lost: %#v", score)
	}
}

func TestWatchDatasetConfinesFutureSourcesAndRejectsUnboundedScripts(t *testing.T) {
	report, _ := scoreFixture()
	d := report.Dataset
	d.Episodes[1].AtSecond = 10
	if d.Validate() == nil {
		t.Fatal("accepted ambiguous episode ordering")
	}
	report, _ = scoreFixture()
	d = report.Dataset
	d.Episodes[0].SourceFile = "../outside.md"
	if d.Validate() == nil {
		t.Fatal("accepted an escaping fixture path")
	}
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("not fixture material"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "source.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadWatchSource(filepath.Join(dir, "dataset.json"), WatchEpisode{SourceFile: "source.md"}); err == nil {
		t.Fatal("followed a fixture symlink")
	}
}
