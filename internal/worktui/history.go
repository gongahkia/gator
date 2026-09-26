package worktui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/workhistory"
)

const maxHistoryItems = 50

type historyItem struct {
	Record   workhistory.Record
	Project  string
	Delivery string
}

type historyDetail struct {
	historyItem
	Artifacts     []ArtifactSummary
	ArtifactIssue string
	Deliveries    []delivery.Record
	Observations  []learning.Observation
	Learnings     []learning.Record
	Review        string
}

func (m *Model) openGlobalHistory() {
	m.section, m.launcher, m.home = "history", false, false
	m.historyDetail, m.sectionNotice = nil, ""
	m.selected = 0
	m.refreshGlobalHistory("")
}

func (m *Model) refreshGlobalHistory(selectedID string) {
	m.historyItems = nil
	if m.config.HistoryStore == nil {
		m.sectionNotice = "Work history is unavailable in this build."
		return
	}
	records, err := m.config.HistoryStore.List(maxHistoryItems)
	if err != nil {
		m.sectionNotice = "Read Work history: " + boundedSectionText(err.Error())
		return
	}
	for _, record := range records {
		if m.historyFilter != "" && record.Status != m.historyFilter {
			continue
		}
		item := historyItem{Record: record, Project: m.historyProject(record.ConversationID)}
		if m.config.DeliveryStore != nil {
			if deliveries, deliveryErr := m.config.DeliveryStore.ListWork(record.ID); deliveryErr == nil {
				item.Delivery = historyDeliveryState(deliveries)
			} else {
				item.Delivery = "unavailable"
			}
		}
		if item.Delivery == "" {
			item.Delivery = "not delivered"
		}
		m.historyItems = append(m.historyItems, item)
	}
	for index, item := range m.historyItems {
		if item.Record.ID == selectedID {
			m.selected = index
			break
		}
	}
}

func (m Model) historyProject(conversationID string) string {
	if conversationID == "" || m.config.SessionStore == nil {
		return ""
	}
	conversation, err := m.config.SessionStore.Load(conversationID)
	if err != nil {
		return ""
	}
	return conversation.SourcePath
}

func (m *Model) selectHistoryItem() {
	if m.selected < 0 || m.selected >= len(m.historyItems) {
		return
	}
	if m.config.HistoryStore == nil {
		m.sectionNotice = "Work history is unavailable in this build."
		return
	}
	item := m.historyItems[m.selected]
	record, err := m.config.HistoryStore.Load(item.Record.ID)
	if err != nil {
		m.sectionNotice = "Load Work: " + boundedSectionText(err.Error())
		return
	}
	detail := historyDetail{historyItem: item}
	detail.Record = record
	if m.config.DeliveryStore != nil {
		detail.Deliveries, err = m.config.DeliveryStore.ListWork(record.ID)
		if err != nil {
			m.sectionNotice = "Load delivery: " + boundedSectionText(err.Error())
			return
		}
		detail.Delivery = historyDeliveryState(detail.Deliveries)
	}
	if m.config.LearningStore != nil {
		detail.Observations, err = m.config.LearningStore.ListObservations(record.ID)
		if err != nil {
			m.sectionNotice = "Load feedback: " + boundedSectionText(err.Error())
			return
		}
		all, listErr := m.config.LearningStore.List()
		if listErr != nil {
			m.sectionNotice = "Load learnings: " + boundedSectionText(listErr.Error())
			return
		}
		for _, candidate := range all {
			if containsHistoryID(candidate.Provenance.WorkIDs, record.ID) {
				detail.Learnings = append(detail.Learnings, candidate)
			}
		}
	}
	detail.Artifacts, detail.ArtifactIssue = loadHistoryArtifacts(record)
	m.historyDetail, m.sectionNotice = &detail, ""
}

func loadHistoryArtifacts(record workhistory.Record) ([]ArtifactSummary, string) {
	if record.Evidence.ArtifactManifestPath == "" {
		return nil, "No retained artifact bundle."
	}
	bundle, err := artifact.OpenBundle(filepath.Dir(record.Evidence.ArtifactManifestPath))
	if err != nil {
		return nil, "Artifact bundle unavailable: " + boundedSectionText(err.Error())
	}
	valid := map[string]bool{}
	for _, file := range bundle.Manifest.Artifacts {
		valid[file.Path] = true
	}
	for _, validation := range bundle.Manifest.Validations {
		if !validation.Passed {
			valid[validation.Path] = false
		}
	}
	items := make([]ArtifactSummary, 0, len(bundle.Manifest.Artifacts))
	for _, file := range bundle.Manifest.Artifacts {
		items = append(items, ArtifactSummary{Path: file.Path, MediaType: file.MediaType, Bytes: file.Bytes, Valid: valid[file.Path]})
	}
	if err := artifact.VerifyBundle(bundle); err != nil {
		return items, "Artifact verification is unavailable: " + boundedSectionText(err.Error())
	}
	return items, ""
}

func containsHistoryID(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func historyDeliveryState(records []delivery.Record) string {
	if len(records) == 0 {
		return "not delivered"
	}
	counts := map[delivery.Status]int{}
	for _, record := range records {
		for _, effect := range record.Effects {
			counts[effect.Status]++
		}
	}
	if counts[delivery.Failed]+counts[delivery.Unknown]+counts[delivery.Pending] > 0 {
		return "needs attention"
	}
	if counts[delivery.Applied] > 0 {
		return "applied"
	}
	return "recorded"
}

func (m *Model) cycleHistoryFilter() {
	filters := []workhistory.Status{"", workhistory.Running, workhistory.Completed, workhistory.Failed}
	for index, value := range filters {
		if value == m.historyFilter {
			m.historyFilter = filters[(index+1)%len(filters)]
			break
		}
	}
	m.selected = 0
	m.refreshGlobalHistory("")
}

func (m *Model) setHistoryFilter(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		m.historyFilter = ""
	case string(workhistory.Running):
		m.historyFilter = workhistory.Running
	case string(workhistory.Completed):
		m.historyFilter = workhistory.Completed
	case string(workhistory.Failed):
		m.historyFilter = workhistory.Failed
	default:
		return errors.New("history status must be all, running, completed, or failed")
	}
	return nil
}

func (m *Model) reviewHistoryDetail() {
	if m.historyDetail == nil {
		return
	}
	path := m.historyDetail.Record.Evidence.ArtifactManifestPath
	if path == "" {
		m.sectionNotice = "This Work has no retained output to review."
		return
	}
	if m.config.BundleAction == nil {
		m.sectionNotice = "Artifact review is unavailable in this build."
		return
	}
	text, err := m.config.BundleAction(BundleActionRequest{Action: "preview", BundlePath: filepath.Dir(path)})
	if err != nil {
		m.sectionNotice = "Review Work: " + boundedSectionText(err.Error())
		return
	}
	m.historyDetail.Review, m.sectionNotice = boundedSectionText(text), ""
}

func (m *Model) resumeHistoryConversation() {
	if m.historyDetail == nil || m.historyDetail.Record.ConversationID == "" {
		m.sectionNotice = "This Work has no retained conversation to reopen."
		return
	}
	if m.config.LoadConversation == nil {
		m.sectionNotice = "Conversation restore is unavailable in this build."
		return
	}
	conversationID := m.historyDetail.Record.ConversationID
	m.section, m.historyDetail, m.sectionNotice = "", nil, ""
	m.restoreConversation(conversationID, "Opened from History.")
}

func (m Model) prepareHistoryRetry() (tea.Model, tea.Cmd) {
	if m.historyDetail == nil {
		return m, nil
	}
	if m.config.BundleAction == nil {
		m.sectionNotice = "Delivery retry is unavailable in this build."
		return m, nil
	}
	if len(m.historyDetail.Deliveries) != 1 {
		m.sectionNotice = "Retry is available when this Work has one selected delivery record."
		return m, nil
	}
	record := m.historyDetail.Deliveries[0]
	eligible := false
	for _, effect := range record.Effects {
		eligible = eligible || effect.Retryable && (effect.Status == delivery.Pending || effect.Status == delivery.Failed)
	}
	if !eligible {
		m.sectionNotice = "This delivery has no known retryable effects. Unknown outcomes require inspection."
		return m, nil
	}
	path := m.historyDetail.Record.Evidence.ArtifactManifestPath
	if path == "" {
		m.sectionNotice = "This Work has no retained output for a delivery retry."
		return m, nil
	}
	m.lastBundle = BundleSummary{Path: filepath.Dir(path)}
	m.section, m.historyDetail, m.sectionNotice = "", nil, ""
	return m.prepareRetry("/retry " + record.ID)
}

func (m Model) historyRetryAvailable() bool {
	if m.historyDetail == nil || m.config.BundleAction == nil || len(m.historyDetail.Deliveries) != 1 || m.historyDetail.Record.Evidence.ArtifactManifestPath == "" {
		return false
	}
	for _, effect := range m.historyDetail.Deliveries[0].Effects {
		if effect.Retryable && (effect.Status == delivery.Pending || effect.Status == delivery.Failed) {
			return true
		}
	}
	return false
}

func historyFilterName(value workhistory.Status) string {
	if value == "" {
		return "all"
	}
	return string(value)
}

func historyStatusLine(record workhistory.Record, project, delivery string) string {
	parts := []string{string(record.Status), string(record.Mode), "verification " + valueOrNone(string(record.VerificationStatus)), "delivery " + delivery}
	if project != "" {
		parts = append(parts, "project "+filepath.Base(project))
	}
	if !record.StartedAt.IsZero() {
		parts = append(parts, record.StartedAt.Local().Format("2006-01-02 15:04"))
	}
	return strings.Join(parts, " · ")
}

func formatHistoryDeliveries(records []delivery.Record) []string {
	lines := make([]string, 0, len(records))
	for _, record := range records[:min(3, len(records))] {
		states := make([]string, 0, len(record.Effects))
		for _, effect := range record.Effects {
			states = append(states, effect.SourcePath+" "+string(effect.Status))
		}
		lines = append(lines, record.ID+" · "+strings.Join(states, ", "))
	}
	if len(records) > 3 {
		lines = append(lines, fmt.Sprintf("+%d more delivery records", len(records)-3))
	}
	return lines
}
