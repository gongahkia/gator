package workrun

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestWorkControlRejectsUnauthorizedAndRedirectedCommands(t *testing.T) {
	state := t.TempDir()
	model := &controlledModel{entered: make(chan struct{}), release: make(chan struct{})}
	operation := (Service{Executor: Executor{Model: model, StateDir: state}}).Start(context.Background(), Request{
		RunID: "protected-control", SourcePath: t.TempDir(), Objective: "Inspect", Mode: action.Inspect, Contract: artifact.InspectionContract(),
	})
	defer operation.Cancel()
	<-model.entered
	root, err := workspace.Open(filepath.Join(state, "gator", "work-control"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := root.ReadRegularFile("protected-control.json", 4096)
	if err != nil {
		t.Fatal(err)
	}
	var control workControlState
	if err := json.Unmarshal(payload, &control); err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Timeout: time.Second}).Post(control.URL+"/stop", "text/plain", nil)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized stop response = %v, %v", response, err)
	}
	_ = response.Body.Close()
	control.URL = "http://example.com"
	changed, _ := json.Marshal(control)
	if err := root.WriteRegularFileAtomic("protected-control.json", changed, 4096); err != nil {
		t.Fatal(err)
	}
	if err := SendWorkControl(context.Background(), state, "protected-control", "stop", ""); err == nil || !strings.Contains(err.Error(), "literal loopback") {
		t.Fatalf("redirected control endpoint was accepted: %v", err)
	}
	if err := root.WriteRegularFileAtomic("protected-control.json", payload, 4096); err != nil {
		t.Fatal(err)
	}
	if err := SendWorkControl(context.Background(), state, "protected-control", "stop", ""); err != nil {
		t.Fatal(err)
	}
	if result := <-operation.Done; result.Err == nil {
		t.Fatal("stopped run completed normally")
	}
}

func TestWorkControlAcrossProcesses(t *testing.T) {
	for _, command := range []string{"steer", "stop"} {
		t.Run(command, func(t *testing.T) {
			state, source := t.TempDir(), t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkControlHelperProcess$")
			child.Env = append(os.Environ(), "GATOR_CONTROL_HELPER=1", "GATOR_CONTROL_STATE="+state, "GATOR_CONTROL_SOURCE="+source)
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := child.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			child.Stderr = &stderr
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			scanner := bufio.NewScanner(stdout)
			if !scanner.Scan() || scanner.Text() != "READY" {
				t.Fatalf("child did not start: %q, %v, %s", scanner.Text(), scanner.Err(), stderr.String())
			}
			root, err := workspace.Open(filepath.Join(state, "gator", "work-control"))
			if err != nil {
				t.Fatal(err)
			}
			controlFile := "cross-process-control.json"
			info, err := os.Stat(filepath.Join(root.Path(), controlFile))
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("control file mode = %v, %v", info, err)
			}
			instruction := ""
			if command == "steer" {
				instruction = "Remember amber"
			}
			if err := SendWorkControl(ctx, state, "cross-process-control", command, instruction); err != nil {
				t.Fatal(err)
			}
			if command == "steer" {
				_, _ = io.WriteString(stdin, "release\n")
			}
			_ = stdin.Close()
			if !scanner.Scan() {
				t.Fatalf("child produced no result: %v, %s", scanner.Err(), stderr.String())
			}
			result := scanner.Text()
			if command == "steer" && !strings.Contains(result, "amber") {
				t.Fatalf("cross-process steering was not applied: %q", result)
			}
			if command == "stop" && result != "STOPPED" {
				t.Fatalf("cross-process stop failed: %q", result)
			}
			if err := child.Wait(); err != nil {
				t.Fatalf("child failed: %v, %s", err, stderr.String())
			}
			if _, err := root.ReadRegularFile(controlFile, 4096); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("completed run retained control state: %v", err)
			}
			if err := SendWorkControl(context.Background(), state, "cross-process-control", command, instruction); err == nil {
				t.Fatal("completed run accepted a control command")
			}
		})
	}
}

func TestWorkControlHelperProcess(t *testing.T) {
	if os.Getenv("GATOR_CONTROL_HELPER") != "1" {
		return
	}
	state, source := os.Getenv("GATOR_CONTROL_STATE"), os.Getenv("GATOR_CONTROL_SOURCE")
	model := &controlledModel{entered: make(chan struct{}), release: make(chan struct{})}
	operation := (Service{Executor: Executor{Model: model, StateDir: state}}).Start(context.Background(), Request{
		RunID: "cross-process-control", SourcePath: source, Objective: "Inspect", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 2,
	})
	<-model.entered
	fmt.Println("READY")
	go func() {
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if line == "release\n" {
			close(model.release)
		}
	}()
	result := <-operation.Done
	if result.Err != nil {
		fmt.Println("STOPPED")
		return
	}
	fmt.Println("FINAL:" + strings.ReplaceAll(result.Outcome.Result.FinalText, "\n", " "))
}
