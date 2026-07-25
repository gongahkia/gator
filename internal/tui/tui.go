package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/engine"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type client struct {
	base string
	http *http.Client
}

func (c client) do(method, path string, body any, output any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, strings.TrimRight(c.base, "/")+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure map[string]string
		_ = json.NewDecoder(response.Body).Decode(&failure)
		return fmt.Errorf("%s", failure["error"])
	}
	if output != nil {
		return json.NewDecoder(response.Body).Decode(output)
	}
	return nil
}

type resultMsg struct {
	runs []domain.Run
	err  error
}
type actionMsg struct {
	run domain.Run
	err error
}
type graphMsg struct {
	run domain.Run
	err error
}
type deploymentMsg struct {
	info       engine.DeploymentInfo
	deployment domain.Deployment
	logs       string
	err        error
}
type providersMsg struct {
	providers []config.Provider
	err       error
}
type tickMsg time.Time

type mode string

const (
	normal     mode = "normal"
	createMode mode = "create"
	reviseMode mode = "revise"
	graphMode  mode = "graph"
	labelMode  mode = "label"
)

type Model struct {
	client        client
	runs          []domain.Run
	selected      int
	node          int
	edge          int
	edgeSource    int
	mode          mode
	input         textinput.Model
	profile       domain.Profile
	providers     []config.Provider
	selections    map[domain.Stage]string
	providerStage int
	message       string
	deploymentLog string
	err           error
	width, height int
}

func Run(apiBase string) error {
	input := textinput.New()
	input.Placeholder = "Describe the app"
	input.CharLimit = 4000
	model := Model{client: client{base: apiBase, http: &http.Client{Timeout: 10 * time.Second}}, input: input, profile: domain.ProfileFullStack, selections: map[domain.Stage]string{}}
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.refresh(), m.fetchProviders(), tick()) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = typed.Width, typed.Height
		return m, nil
	case resultMsg:
		m.err = typed.err
		if typed.err == nil {
			m.runs = typed.runs
			if m.selected >= len(m.runs) {
				m.selected = max(0, len(m.runs)-1)
			}
		}
		return m, nil
	case actionMsg:
		m.err = typed.err
		if typed.err == nil {
			m.replace(typed.run)
			m.message = "Action accepted."
		}
		return m, nil
	case graphMsg:
		m.err = typed.err
		if typed.err == nil {
			m.replace(typed.run)
			m.message = "Graph saved."
			m.mode = normal
		}
		return m, nil
	case providersMsg:
		m.err = typed.err
		if typed.err == nil {
			m.providers = typed.providers
			m.ensureSelections()
		}
		return m, nil
	case deploymentMsg:
		m.err = typed.err
		if typed.err == nil {
			if typed.logs != "" {
				m.deploymentLog = typed.logs
				m.message = "Deployment logs loaded."
			} else if typed.info.Deployment.RunID != "" {
				m.message = "Deployment: " + typed.info.Deployment.Status + " (" + fmt.Sprint(len(typed.info.Runtime.Services)) + " services)."
			} else {
				m.message = "Deployment: " + typed.deployment.Status
			}
		}
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.refresh(), tick())
	case tea.KeyMsg:
		return m.key(typed)
	}
	if m.mode == createMode || m.mode == reviseMode || m.mode == labelMode {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) key(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	value := key.String()
	if value == "ctrl+c" || value == "q" {
		return m, tea.Quit
	}
	if m.mode == createMode {
		return m.createKey(key)
	}
	if m.mode == reviseMode {
		return m.reviseKey(key)
	}
	if m.mode == labelMode {
		return m.labelKey(key)
	}
	if m.mode == graphMode {
		return m.graphKey(value)
	}
	switch value {
	case "r":
		return m, m.refresh()
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected+1 < len(m.runs) {
			m.selected++
		}
	case "n":
		m.mode = createMode
		m.input.SetValue("")
		m.input.Focus()
		m.message = "New run: choose profile with p; enter submits."
	case "p":
		m.profile = nextProfile(m.profile)
		m.message = "Profile: " + string(m.profile)
	case "a":
		if run, ok := m.run(); ok && run.Status == domain.StatusAwaiting {
			return m, m.action(run.ID, domain.ApprovalApprove, "")
		}
	case "v":
		if run, ok := m.run(); ok && run.Status == domain.StatusAwaiting && run.Stage == domain.StagePlanner {
			m.mode = reviseMode
			m.input.SetValue("")
			m.input.Placeholder = "Planner revision feedback"
			m.input.Focus()
		}
	case "t":
		if run, ok := m.run(); ok && (run.Status == domain.StatusFailed || run.Status == domain.StatusInterrupted) {
			return m, m.action(run.ID, domain.ApprovalRetry, "")
		}
	case "x":
		if run, ok := m.run(); ok && run.Status != domain.StatusAbandoned && run.Status != domain.StatusCompleted {
			return m, m.action(run.ID, domain.ApprovalAbandon, "")
		}
	case "i":
		if run, ok := m.run(); ok {
			return m, m.deploymentStatus(run.ID)
		}
	case "l":
		if run, ok := m.run(); ok {
			return m, m.deploymentLogs(run.ID)
		}
	case "S":
		if run, ok := m.run(); ok {
			return m, m.deploymentAction(run.ID, "start")
		}
	case "P":
		if run, ok := m.run(); ok {
			return m, m.deploymentAction(run.ID, "stop")
		}
	case "D":
		if run, ok := m.run(); ok {
			return m, m.deleteDeployment(run.ID)
		}
	case "g":
		if run, ok := m.run(); ok && run.Stage == domain.StagePlanner && run.Status == domain.StatusAwaiting {
			m.mode = graphMode
			m.node = 0
			m.edgeSource = -1
			m.edge = 0
			m.message = "Graph: n add · d delete node · c select/connect edge · [/] edge · x delete edge · HJKL move · e label · s save."
		}
	}
	return m, nil
}

func (m Model) createKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	value := key.String()
	switch value {
	case "esc":
		m.mode = normal
		m.input.Blur()
		return m, nil
	case "p":
		m.profile = nextProfile(m.profile)
		return m, nil
	case "tab":
		m.providerStage = (m.providerStage + 1) % len(stageChoices)
		return m, nil
	case "left", "h":
		m.cycleProvider(-1)
		return m, nil
	case "right", "l":
		m.cycleProvider(1)
		return m, nil
	case "enter":
		prompt := strings.TrimSpace(m.input.Value())
		if prompt == "" {
			m.err = fmt.Errorf("prompt is required")
			return m, nil
		}
		m.mode = normal
		m.input.Blur()
		return m, m.create(prompt)
	}
	var command tea.Cmd
	m.input, command = m.input.Update(key)
	return m, command
}
func (m Model) reviseKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	value := key.String()
	switch value {
	case "esc":
		m.mode = normal
		m.input.Blur()
		return m, nil
	case "enter":
		if run, ok := m.run(); ok {
			feedback := strings.TrimSpace(m.input.Value())
			if feedback == "" {
				m.err = fmt.Errorf("feedback is required")
				return m, nil
			}
			m.mode = normal
			m.input.Blur()
			return m, m.action(run.ID, domain.ApprovalRevise, feedback)
		}
	}
	var command tea.Cmd
	m.input, command = m.input.Update(key)
	return m, command
}
func (m Model) labelKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	value := key.String()
	switch value {
	case "esc":
		m.mode = graphMode
		m.input.Blur()
		return m, nil
	case "enter":
		if run, ok := m.run(); ok && m.node < len(run.Graph.Nodes) {
			run.Graph.Nodes[m.node].Label = strings.TrimSpace(m.input.Value())
			m.replace(run)
			m.mode = graphMode
			m.input.Blur()
		}
	}
	var command tea.Cmd
	m.input, command = m.input.Update(key)
	return m, command
}
func (m Model) graphKey(value string) (tea.Model, tea.Cmd) {
	run, ok := m.run()
	if !ok {
		m.mode = normal
		return m, nil
	}
	switch value {
	case "esc":
		if m.edgeSource >= 0 {
			m.edgeSource = -1
			m.message = "Edge selection cancelled."
		} else {
			m.mode = normal
		}
	case "up", "k":
		if m.node > 0 {
			m.node--
		}
	case "down", "j":
		if m.node+1 < len(run.Graph.Nodes) {
			m.node++
		}
	case "e":
		if m.node < len(run.Graph.Nodes) {
			m.mode = labelMode
			m.input.SetValue(run.Graph.Nodes[m.node].Label)
			m.input.Placeholder = "Node label"
			m.input.Focus()
		}
	case "n":
		id := fmt.Sprintf("tool-optional-%d", len(run.Graph.Nodes)+1)
		node := domain.GraphNode{ID: id, Label: "Optional tool", Kind: "tool", Optional: true, Data: map[string]any{"x": 0, "y": 0}}
		output := len(run.Graph.Nodes) - 1
		run.Graph.Nodes = append(run.Graph.Nodes[:output], append([]domain.GraphNode{node}, run.Graph.Nodes[output:]...)...)
		previous, next := run.Graph.Nodes[output-1].ID, run.Graph.Nodes[output+1].ID
		run.Graph.Edges = removeEdge(run.Graph.Edges, previous, next)
		run.Graph.Edges = append(run.Graph.Edges, edge(previous, id), edge(id, next))
		m.replace(run)
		m.node = output
	case "d":
		if m.node < len(run.Graph.Nodes) && run.Graph.Nodes[m.node].Optional {
			removed := run.Graph.Nodes[m.node].ID
			predecessors, successors := graphNeighbours(run.Graph, removed)
			run.Graph.Nodes = append(run.Graph.Nodes[:m.node], run.Graph.Nodes[m.node+1:]...)
			run.Graph.Edges = removeNodeEdges(run.Graph.Edges, removed)
			for _, source := range predecessors {
				for _, target := range successors {
					if source != target && !hasEdge(run.Graph.Edges, source, target) {
						run.Graph.Edges = append(run.Graph.Edges, edge(source, target))
					}
				}
			}
			m.replace(run)
			m.node = max(0, m.node-1)
		}
	case "c":
		if m.edgeSource < 0 {
			m.edgeSource = m.node
			m.message = "Select edge target, then press c. Esc cancels."
		} else if m.edgeSource == m.node {
			m.err = fmt.Errorf("an edge cannot target itself")
		} else {
			source, target := run.Graph.Nodes[m.edgeSource].ID, run.Graph.Nodes[m.node].ID
			if hasEdge(run.Graph.Edges, source, target) {
				m.err = fmt.Errorf("edge already exists")
			} else {
				run.Graph.Edges = append(run.Graph.Edges, edge(source, target))
				if err := run.Graph.Validate(); err != nil {
					run.Graph.Edges = run.Graph.Edges[:len(run.Graph.Edges)-1]
					m.err = err
				} else {
					m.replace(run)
					m.message = "Edge added."
				}
			}
			m.edgeSource = -1
		}
	case "[":
		if len(run.Graph.Edges) > 0 {
			m.edge = (m.edge - 1 + len(run.Graph.Edges)) % len(run.Graph.Edges)
		}
	case "]":
		if len(run.Graph.Edges) > 0 {
			m.edge = (m.edge + 1) % len(run.Graph.Edges)
		}
	case "x":
		if len(run.Graph.Edges) > 0 {
			run.Graph.Edges = append(run.Graph.Edges[:m.edge], run.Graph.Edges[m.edge+1:]...)
			m.edge = min(m.edge, max(0, len(run.Graph.Edges)-1))
			m.replace(run)
			m.message = "Edge deleted."
		}
	case "H", "J", "K", "L":
		if m.node < len(run.Graph.Nodes) {
			moveNode(&run.Graph.Nodes[m.node], value)
			m.replace(run)
		}
	case "s":
		return m, m.saveGraph(run)
	}
	return m, nil
}

func (m Model) View() string {
	var body strings.Builder
	body.WriteString(titleStyle.Render("Norbot") + "  " + mutedStyle.Render("provider-agnostic, operator-gated app builder") + "\n")
	body.WriteString(mutedStyle.Render("n new · p profile · a approve · v revise · t retry · x abandon · g graph · i status · l logs · S start · P stop · D delete · r refresh · q quit") + "\n\n")
	if m.err != nil {
		body.WriteString(errorStyle.Render("error: "+m.err.Error()) + "\n")
	}
	if m.message != "" {
		body.WriteString(mutedStyle.Render(m.message) + "\n")
	}
	if m.mode == createMode {
		stage := stageChoices[m.providerStage]
		body.WriteString(titleStyle.Render("New "+string(m.profile)+" run") + "\n" + m.input.View() + "\n")
		body.WriteString(mutedStyle.Render("p profile · tab stage · ←/→ provider · selected "+string(stage)+": "+m.selections[stage]) + "\n")
		return body.String()
	}
	if m.mode == reviseMode {
		body.WriteString(titleStyle.Render("Planner revision") + "\n" + m.input.View() + "\n")
		return body.String()
	}
	if m.mode == labelMode {
		body.WriteString(titleStyle.Render("Edit node label") + "\n" + m.input.View() + "\n")
	}
	if len(m.runs) == 0 {
		body.WriteString("No runs. Press n to create one.\n")
		return body.String()
	}
	body.WriteString(titleStyle.Render("Runs") + "\n")
	for i, run := range m.runs {
		prefix := "  "
		if i == m.selected {
			prefix = selectedStyle.Render("> ")
		}
		body.WriteString(fmt.Sprintf("%s%s  %-20s %-18s %s\n", prefix, run.ID, run.Stage, run.Status, run.Profile))
	}
	if run, ok := m.run(); ok {
		body.WriteString("\n" + titleStyle.Render("Workflow graph") + "\n" + drawGraph(run.Graph, m.node, m.mode == graphMode) + "\n")
		if m.mode == graphMode && len(run.Graph.Edges) > 0 {
			body.WriteString(mutedStyle.Render("Edge "+fmt.Sprintf("%d/%d: %s → %s", m.edge+1, len(run.Graph.Edges), run.Graph.Edges[m.edge].Source, run.Graph.Edges[m.edge].Target)) + "\n")
		}
		body.WriteString(mutedStyle.Render("Run "+run.ID+" · provider "+run.Providers[run.Stage]) + "\n")
		if run.FailureReason != "" {
			body.WriteString(errorStyle.Render(run.FailureReason) + "\n")
		}
		if m.deploymentLog != "" {
			body.WriteString("\n" + titleStyle.Render("Deployment logs") + "\n" + tailLines(m.deploymentLog, 12) + "\n")
		}
	}
	return body.String()
}

func drawGraph(graph domain.Graph, selected int, editing bool) string {
	labels := make([]string, 0, len(graph.Nodes))
	for i, node := range graph.Nodes {
		label := "[" + node.Kind + ": " + node.Label + "]"
		if node.Data != nil {
			x, xOK := node.Data["x"].(int)
			y, yOK := node.Data["y"].(int)
			if xOK || yOK {
				label += fmt.Sprintf(" (%d,%d)", x, y)
			}
		}
		if editing && i == selected {
			label = selectedStyle.Render("▸ " + label)
		}
		labels = append(labels, label)
	}
	edges := make([]string, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		edges = append(edges, edge.Source+" → "+edge.Target)
	}
	return strings.Join(labels, "  ") + "\n" + mutedStyle.Render(strings.Join(edges, " · "))
}

func edge(source, target string) domain.GraphEdge {
	return domain.GraphEdge{ID: source + "-to-" + target, Source: source, Target: target}
}
func hasEdge(edges []domain.GraphEdge, source, target string) bool {
	for _, item := range edges {
		if item.Source == source && item.Target == target {
			return true
		}
	}
	return false
}
func removeEdge(edges []domain.GraphEdge, source, target string) []domain.GraphEdge {
	result := edges[:0]
	for _, item := range edges {
		if item.Source != source || item.Target != target {
			result = append(result, item)
		}
	}
	return result
}
func removeNodeEdges(edges []domain.GraphEdge, node string) []domain.GraphEdge {
	result := edges[:0]
	for _, item := range edges {
		if item.Source != node && item.Target != node {
			result = append(result, item)
		}
	}
	return result
}
func graphNeighbours(graph domain.Graph, node string) ([]string, []string) {
	predecessors, successors := []string{}, []string{}
	for _, item := range graph.Edges {
		if item.Target == node {
			predecessors = append(predecessors, item.Source)
		}
		if item.Source == node {
			successors = append(successors, item.Target)
		}
	}
	return predecessors, successors
}
func moveNode(node *domain.GraphNode, direction string) {
	if node.Data == nil {
		node.Data = map[string]any{}
	}
	x, _ := node.Data["x"].(int)
	y, _ := node.Data["y"].(int)
	switch direction {
	case "H":
		x--
	case "L":
		x++
	case "K":
		y--
	case "J":
		y++
	}
	node.Data["x"], node.Data["y"] = x, y
}
func linearEdges(nodes []domain.GraphNode) []domain.GraphEdge {
	edges := make([]domain.GraphEdge, 0, max(0, len(nodes)-1))
	for i := 0; i+1 < len(nodes); i++ {
		edges = append(edges, domain.GraphEdge{ID: nodes[i].ID + "-to-" + nodes[i+1].ID, Source: nodes[i].ID, Target: nodes[i+1].ID})
	}
	return edges
}
func nextProfile(profile domain.Profile) domain.Profile {
	profiles := []domain.Profile{domain.ProfileFrontend, domain.ProfileFullStack, domain.ProfileAgentic}
	for i, current := range profiles {
		if current == profile {
			return profiles[(i+1)%len(profiles)]
		}
	}
	return domain.ProfileFullStack
}
func (m *Model) run() (domain.Run, bool) {
	if m.selected < 0 || m.selected >= len(m.runs) {
		return domain.Run{}, false
	}
	return m.runs[m.selected], true
}
func (m *Model) replace(next domain.Run) {
	for i, run := range m.runs {
		if run.ID == next.ID {
			m.runs[i] = next
			return
		}
	}
	m.runs = append(m.runs, next)
	sort.Slice(m.runs, func(i, j int) bool { return m.runs[i].UpdatedAt.After(m.runs[j].UpdatedAt) })
}
func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		var runs []domain.Run
		err := m.client.do(http.MethodGet, "/api/runs", nil, &runs)
		return resultMsg{runs, err}
	}
}
func (m Model) create(prompt string) tea.Cmd {
	return func() tea.Msg {
		var run domain.Run
		providers := make(map[domain.Stage]string, len(m.selections))
		for stage, providerID := range m.selections {
			providers[stage] = providerID
		}
		err := m.client.do(http.MethodPost, "/api/runs", engine.CreateRunInput{Prompt: prompt, Profile: m.profile, Providers: providers}, &run)
		return actionMsg{run, err}
	}
}

var stageChoices = []domain.Stage{domain.StagePlanner, domain.StageBuilder, domain.StageVerifier}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg(time.Now()) })
}

func (m Model) fetchProviders() tea.Cmd {
	return func() tea.Msg {
		var providers []config.Provider
		err := m.client.do(http.MethodGet, "/api/providers", nil, &providers)
		return providersMsg{providers: providers, err: err}
	}
}

func (m *Model) ensureSelections() {
	for _, stage := range stageChoices {
		if m.selections[stage] != "" {
			continue
		}
		for _, option := range m.providers {
			for _, supported := range option.Stages {
				if supported == stage {
					m.selections[stage] = option.ID
					break
				}
			}
			if m.selections[stage] != "" {
				break
			}
		}
	}
}

func (m *Model) cycleProvider(delta int) {
	stage := stageChoices[m.providerStage]
	candidates := []string{}
	for _, option := range m.providers {
		for _, supported := range option.Stages {
			if supported == stage {
				candidates = append(candidates, option.ID)
			}
		}
	}
	if len(candidates) == 0 {
		return
	}
	current := 0
	for index, id := range candidates {
		if id == m.selections[stage] {
			current = index
			break
		}
	}
	m.selections[stage] = candidates[(current+delta+len(candidates))%len(candidates)]
}
func (m Model) action(id string, action domain.ApprovalAction, feedback string) tea.Cmd {
	return func() tea.Msg {
		var run domain.Run
		err := m.client.do(http.MethodPost, "/api/runs/"+id+"/approval", engine.ApprovalInput{Action: action, Feedback: feedback}, &run)
		return actionMsg{run, err}
	}
}
func (m Model) saveGraph(run domain.Run) tea.Cmd {
	return func() tea.Msg {
		var saved domain.Run
		err := m.client.do(http.MethodPut, "/api/runs/"+run.ID+"/graph", run.Graph, &saved)
		return graphMsg{saved, err}
	}
}
func (m Model) deploymentStatus(id string) tea.Cmd {
	return func() tea.Msg {
		var info engine.DeploymentInfo
		err := m.client.do(http.MethodGet, "/api/runs/"+id+"/deployment", nil, &info)
		return deploymentMsg{info: info, err: err}
	}
}
func (m Model) deploymentLogs(id string) tea.Cmd {
	return func() tea.Msg {
		var out struct {
			Logs string `json:"logs"`
		}
		err := m.client.do(http.MethodGet, "/api/runs/"+id+"/deployment/logs?lines=200", nil, &out)
		return deploymentMsg{logs: out.Logs, err: err}
	}
}
func (m Model) deploymentAction(id, action string) tea.Cmd {
	return func() tea.Msg {
		if action == "start" {
			var info engine.DeploymentInfo
			err := m.client.do(http.MethodPost, "/api/runs/"+id+"/deployment/start", nil, &info)
			return deploymentMsg{info: info, err: err}
		}
		var deployment domain.Deployment
		err := m.client.do(http.MethodPost, "/api/runs/"+id+"/deployment/"+action, nil, &deployment)
		return deploymentMsg{deployment: deployment, err: err}
	}
}
func (m Model) deleteDeployment(id string) tea.Cmd {
	return func() tea.Msg {
		var deployment domain.Deployment
		err := m.client.do(http.MethodDelete, "/api/runs/"+id+"/deployment", nil, &deployment)
		return deploymentMsg{deployment: deployment, err: err}
	}
}
func tailLines(value string, count int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}
