package tui

import (
	"context"
	"crypto/sha256"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/review"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/terminal"
	"github.com/gongahkia/gator/internal/tools"
)

type screen uint8

const (
	composeScreen screen = iota
	attachmentConfirmScreen
	runningScreen
	terminalScreen
	reviewScreen
	transcriptScreen
	helpScreen
	recentScreen
	threadScreen
	effortScreen
	extensionUIScreen
	localModelsScreen
	managementScreen
	doctorScreen
	runOptionsScreen
	reviewWebScreen
)

type field uint8

const (
	taskField field = iota
	verificationField
	providerField
	modelField
)

type drawerSection uint8

const (
	drawerThreads drawerSection = iota
	drawerRuntime
	drawerActivity
	drawerReview
)

type vimMode uint8

const (
	vimOff vimMode = iota
	vimNormal
	vimInsert
)

type noticeKind uint8

const (
	noticeInfo noticeKind = iota
	noticeError
	noticeSuccess
)

type notice struct {
	text string
	kind noticeKind
}

type timelineEntry struct {
	step   int
	text   string
	detail string
	kind   agent.EventKind
}

type chatAuthor uint8

const (
	chatSystem chatAuthor = iota
	chatUser
	chatAgent
	chatTool
)

// chatEntry is local to the active TUI. The durable session remains the
// source of truth for resume; this is the readable terminal conversation.
type chatEntry struct {
	author    chatAuthor
	text      string
	detail    string
	streaming bool
	isError   bool
}

type queuedInputKind uint8

const (
	queuedPrompt queuedInputKind = iota
	queuedCommand
)

// queuedInput is intentionally TUI-local. A queued instruction is not part of
// a retained session until it actually begins a run.
type queuedInput struct {
	kind queuedInputKind
	text string
}

type attachmentPreview struct {
	name      string
	mediaType string
	bytes     int
	digest    [sha256.Size]byte
}

type commandApprovalRequest struct {
	argv  []string
	reply chan tools.CommandDecision
}

type pendingCommandApproval struct {
	argv  []string
	reply chan tools.CommandDecision
}

type executionStream struct {
	events    chan agent.Event
	done      chan executionDone
	cancel    context.CancelFunc
	steering  chan string
	approvals chan commandApprovalRequest
	terminals chan terminal.Attachment
}

type executionDone struct {
	outcome gatorrun.Outcome
	err     error
}

type agentEventMsg struct {
	event agent.Event
}

type commandApprovalMsg struct {
	argv  []string
	reply chan tools.CommandDecision
}

type terminalManagerMsg struct {
	attachment terminal.Attachment
}

type executionDoneMsg struct {
	done executionDone
}

type oauthLoginDoneMsg struct {
	provider string
	err      error
}

type connectDoneMsg struct {
	provider string
	err      error
}

type delegatedRunDoneMsg struct {
	runtime string
	output  string
	err     error
}

type openCodeCommandDoneMsg struct {
	action   string
	provider string
	err      error
}

type updateStatusMsg struct {
	status UpdateStatus
	err    error
}

type diffLoadedMsg struct {
	diff      string
	truncated bool
	err       error
}

type reviewLoadedMsg struct {
	snapshot review.Snapshot
	err      error
}

type reviewMutationDoneMsg struct {
	err error
}

type activityTickMsg struct{}
