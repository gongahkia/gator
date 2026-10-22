package workapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/worksession"
)

func (m Model) retainsRequests() bool { return m.config.SessionStore != nil && m.conversation != "" }

func copyRequests(j worksession.RequestJournal) worksession.RequestJournal {
	j.Requests = append([]worksession.Request(nil), j.Requests...)
	return j
}

func (m *Model) saveRequests(j worksession.RequestJournal) error {
	// Keep bounded terminal receipts for diagnosis. Pending work is never pruned.
	for len(j.Requests) > worksession.MaxRetainedRequests {
		removed := false
		for i, r := range j.Requests {
			if !r.Pending() {
				j.Requests = append(j.Requests[:i], j.Requests[i+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			return errors.New("too many retained pending messages")
		}
	}
	next, err := m.config.SessionStore.SaveRequests(m.conversation, j)
	if err != nil {
		m.requestsIssue = "Couldn't save message state: " + err.Error()
		return errors.New(m.requestsIssue)
	}
	m.requests = next
	return nil
}

func (m *Model) restoreRequests() {
	m.requests, m.activeRequest, m.nextRequest = worksession.RequestJournal{Version: 1}, "", ""
	m.requestsIssue, m.requestsRecovered = "", false
	if !m.retainsRequests() {
		return
	}
	j, err := m.config.SessionStore.LoadRequests(m.conversation)
	if err != nil {
		m.requestsIssue = "Couldn't restore messages: " + err.Error()
		return
	}
	m.requests = j
	lease, err := m.config.SessionStore.AcquireExecution(m.conversation)
	if err != nil {
		if errors.Is(err, filelock.ErrWouldBlock) {
			m.requestsIssue = "This chat is still working in another session. Reopen it after that work finishes."
		} else {
			m.requestsIssue = "Couldn't check interrupted work: " + err.Error()
		}
		return
	}
	defer lease.Close()
	changed := false
	for i := range j.Requests {
		r := &j.Requests[i]
		if r.State == "running" {
			r.State, r.UpdatedAt = "interrupted", time.Now().UTC()
			r.Issue = "The session ended before completion was confirmed. Check retained work before sending again."
			changed = true
		}
		if !r.Pending() {
			continue
		}
		var options RunOptions
		if err := json.Unmarshal(r.Options, &options); err != nil {
			m.requestsIssue = "Couldn't restore the settings of a retained message."
			return
		}
		m.queue = append(m.queue, queuedRun{id: r.Edit, prompt: r.Text, options: retainedRunOptions(options), requestID: r.ID, interrupted: r.State == "interrupted"})
		m.queueSequence = max(m.queueSequence, r.Edit)
	}
	if changed {
		_ = m.saveRequests(j)
	}
	m.requestsRecovered = len(m.queue) > 0
}

// persistQueue commits the whole edit/reorder/removal before exposing it. Its
// generation prevents stale controls in another window from restoring consumed
// messages. Captured options and upload references travel with each request.
func (m *Model) persistQueue(queue []queuedRun) error {
	if !m.retainsRequests() {
		m.queue = queue
		return nil
	}
	if m.requestsIssue != "" {
		return errors.New(m.requestsIssue)
	}
	j := copyRequests(m.requests)
	byID := map[string]worksession.Request{}
	for _, r := range j.Requests {
		byID[r.ID] = r
	}
	next := make([]worksession.Request, 0, len(queue))
	queue = append([]queuedRun(nil), queue...)
	for i := range queue {
		q := &queue[i]
		options, err := json.Marshal(retainedRunOptions(q.options))
		if err != nil {
			return err
		}
		r, exists := byID[q.requestID]
		if !exists {
			r, err = worksession.NewRequest(q.prompt, options, time.Now())
			if err != nil {
				return err
			}
			q.requestID = r.ID
			r.ClientID = m.submissionID
		}
		r.Text, r.Edit, r.Options, r.UpdatedAt = q.prompt, q.id, options, time.Now().UTC()
		next = append(next, r)
	}
	j.Requests = nil
	for _, r := range m.requests.Requests {
		if r.State == "queued" || r.State == "interrupted" {
			kept := false
			for _, q := range queue {
				if q.requestID == r.ID {
					kept = true
					break
				}
			}
			if kept {
				continue
			}
			r.State, r.UpdatedAt = "discarded", time.Now().UTC()
		}
		j.Requests = append(j.Requests, r)
	}
	j.Requests = append(j.Requests, next...)
	if err := m.saveRequests(j); err != nil {
		return err
	}
	m.queue = queue
	return nil
}

func (m *Model) admitRun(prompt string, options RunOptions) (func(), error) {
	if !m.retainsRequests() {
		return func() {}, nil
	}
	if m.requestsIssue != "" {
		return nil, errors.New(m.requestsIssue)
	}
	lease, err := m.config.SessionStore.AcquireExecution(m.conversation)
	if err != nil {
		return nil, fmt.Errorf("this chat already has active work or cannot acquire ownership: %w", err)
	}
	release := func() { _ = lease.Close() }
	j := copyRequests(m.requests)
	index := -1
	for i, r := range j.Requests {
		if r.ID == m.nextRequest {
			index = i
			break
		}
	}
	if index < 0 {
		if m.nextRequest != "" {
			release()
			return nil, errors.New("this queued message has changed; reopen the chat")
		}
		pending := 0
		for _, r := range j.Requests {
			if r.Pending() {
				pending++
			}
		}
		if pending >= 17 {
			release()
			return nil, errors.New("review or remove a retained message before sending another")
		}
		data, err := json.Marshal(retainedRunOptions(options))
		if err != nil {
			release()
			return nil, err
		}
		r, err := worksession.NewRequest(prompt, data, time.Now())
		if err != nil {
			release()
			return nil, err
		}
		j.Requests = append(j.Requests, r)
		index = len(j.Requests) - 1
		j.Requests[index].ClientID = m.submissionID
	}
	consumed := m.nextRequest
	if j.Requests[index].State == "interrupted" {
		prior := &j.Requests[index]
		retry, err := worksession.NewRequest(prompt, prior.Options, time.Now())
		if err != nil {
			release()
			return nil, err
		}
		retry.RetryOf = prior.ID
		prior.State, prior.UpdatedAt = "retried", time.Now().UTC()
		j.Requests = append(j.Requests, retry)
		index = len(j.Requests) - 1
	}
	j.Requests[index].State, j.Requests[index].UpdatedAt = "running", time.Now().UTC()
	if err := m.saveRequests(j); err != nil {
		release()
		return nil, err
	}
	m.activeRequest = j.Requests[index].ID
	for i, q := range m.queue {
		if q.requestID == consumed {
			m.queue = append(append([]queuedRun(nil), m.queue[:i]...), m.queue[i+1:]...)
			break
		}
	}
	m.nextRequest = ""
	return release, nil
}

func (m *Model) finishRequest(value runDone, stopped bool) {
	if !m.retainsRequests() || m.activeRequest == "" {
		return
	}
	j := copyRequests(m.requests)
	for i := range j.Requests {
		r := &j.Requests[i]
		if r.ID != m.activeRequest {
			continue
		}
		r.State = "completed"
		if value.Error != "" {
			r.State = "failed"
		}
		if stopped {
			r.State = "stopped"
		}
		r.WorkID, r.RevisionID = m.lastWorkID, value.RevisionID
		r.UpdatedAt = time.Now().UTC()
		// Full model/provider errors remain in their existing evidence stores.
		// The admission receipt only needs a bounded diagnostic.
		r.Issue = value.Error
		if len(r.Issue) > 4096 {
			r.Issue = r.Issue[:4096]
		}
		break
	}
	_ = m.saveRequests(j)
	m.activeRequest = ""
}

func (m Model) runQueued(index int) (tea.Model, tea.Cmd) {
	next := m.queue[index]
	if m.retainsRequests() {
		m.nextRequest = next.requestID
	} else {
		m.queue = append(m.queue[:index], m.queue[index+1:]...)
	}
	m.home = false
	m.messages = append(m.messages, message{role: "You", text: next.prompt})
	m.running = true
	m.status = "Working…"
	next.options.PreviousArtifacts = m.previousArtifactPaths()
	return m.startWork(m.source, m.conversation, next.prompt, next.options)
}

func (m *Model) admissionFailed(err error) {
	m.running = false
	m.bridgeNotice = "Message not sent: " + err.Error()
	if m.bridgeUploadResult != nil {
		m.bridgeUploadResult.Error = m.bridgeNotice
	}
	if m.bridgeQueueResult != nil {
		m.bridgeQueueResult.Error = m.bridgeNotice
	}
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "You" {
		m.messages = m.messages[:len(m.messages)-1]
	}
}
