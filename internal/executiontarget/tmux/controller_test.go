// Copyright 2026 gongahkia. Apache-2.0; see third_party/onibi/LICENSE.
// Adapted from Onibi's low-level attach/render test patterns; modified by Gator.
package tmux

import (
	"context"
	"errors"
	"github.com/gongahkia/gator/internal/executiontarget"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type fakeProbe struct{ socket, server, pane string }

func (p *fakeProbe) Socket(string) (string, error) { return p.socket, nil }
func (p *fakeProbe) Process(id int) (string, error) {
	if id == 10 {
		return p.server, nil
	}
	return p.pane, nil
}

type fakeRunner struct {
	afterVersion func()
	calls        [][]string
	row          string
	output       []byte
	capture      func()
	err          error
}

func (r *fakeRunner) Run(_ context.Context, _ string, a []string, _ int) (Result, error) {
	r.calls = append(r.calls, a)
	if len(a) == 1 && a[0] == "-V" {
		if r.afterVersion != nil {
			r.afterVersion()
		}
		return Result{Bytes: []byte("tmux 3.8\n")}, nil
	}
	if r.err != nil {
		return Result{}, r.err
	}
	command := a[len(a)-1]
	if strings.Contains(command, "capture-pane") {
		if r.capture != nil {
			r.capture()
		}
		return Result{Bytes: append([]byte("GATOR_CAPTURE_OK\n"), r.output...)}, nil
	}
	return Result{Bytes: []byte(r.row)}, nil
}
func fixture() (*Controller, *fakeRunner, *fakeProbe) {
	r := &fakeRunner{row: "10|100|$1|101|@3|%4|11|0|3.8|24|1000\n", output: []byte("safe\n")}
	p := &fakeProbe{"socket-generation", "server-start", "pane-start"}
	return &Controller{Config: Config{Executable: "/trusted/tmux", Socket: "/private/mux/socket", ReadConfigReviewed: true, Review: &Review{SocketEvidence: "socket-generation", ExecutableDigest: "digest", Reviewer: executiontarget.Actor{ID: "local", Kind: executiontarget.Human, Origin: executiontarget.Local}, ReviewedAt: time.Now().Add(-time.Second), Scope: ReadTrustScope}}, Runner: r, Probe: p}, r, p
}
func TestExactNativeIdentityAndReadCommands(t *testing.T) {
	c, r, _ := fixture()
	n, e := c.Resolve(context.Background(), "%4")
	if e != nil {
		t.Fatal(e)
	}
	r.row = strings.Replace(r.row, "@3", "@9", 1)
	v, e := c.Capture(context.Background(), n, 100)
	if e != nil || v.Text != "safe" || !v.Truncated {
		t.Fatalf("%+v %v", v, e)
	}
	for _, a := range r.calls {
		if len(a) == 1 {
			continue
		}
		if a[0] != "-N" || a[2] != os.DevNull || a[5] != "if-shell" || a[6] != "-F" {
			t.Fatalf("unsafe argv: %q", a)
		}
		for _, bad := range []string{"send-keys", "select-pane", "resize-pane", "attach-session", "new-session", "set-hook"} {
			if strings.Contains(strings.Join(a, " "), bad) {
				t.Fatal(bad)
			}
		}
	}
	if _, e = c.Resolve(context.Background(), "%4; send-keys x"); e == nil {
		t.Fatal("accepted native injection")
	}
}
func TestIdentityReuseAndCaptureRaces(t *testing.T) {
	for _, what := range []string{"server", "socket", "process", "session", "pane", "disappear"} {
		t.Run(what, func(t *testing.T) {
			c, r, p := fixture()
			n, _ := c.Resolve(context.Background(), "%4")
			r.capture = func() {
				switch what {
				case "server":
					p.server = "replacement"
				case "socket":
					p.socket = "replacement"
				case "process":
					p.pane = "replacement"
				case "session":
					r.row = strings.Replace(r.row, "$1", "$2", 1)
				case "pane":
					r.row = strings.Replace(r.row, "%4", "%5", 1)
				case "disappear":
					r.row = ""
				}
			}
			if _, e := c.Capture(context.Background(), n, 100); e == nil {
				t.Fatal("misattributed output released")
			}
		})
	}
}
func TestBoundsAndConservativeText(t *testing.T) {
	cases := []struct{ in, want string }{{"\x1b[31mred\x1b[0m", "red"}, {"x\x1b]0;secret\aok", "xok"}, {"a\x1bPprivate\x1b\\b", "ab"}, {"<script>prompt injection</script>", "<script>prompt injection</script>"}, {"a\x00\x07b", "ab"}}
	for _, c := range cases {
		out, _ := PlainTail([]byte(c.in), 100, MaxBytes)
		if out != c.want {
			t.Fatalf("%q -> %q", c.in, out)
		}
	}
	out, cut := PlainTail([]byte(strings.Repeat("猫", 10000)), 200, MaxBytes)
	if !cut || !utf8.ValidString(out) || len(out) > MaxBytes {
		t.Fatal("unsafe UTF8 bound")
	}
	out, cut = PlainTail([]byte("one\ntwo\nthree\n"), 2, MaxBytes)
	if out != "two\nthree" || !cut {
		t.Fatal(out, cut)
	}
	c, _, _ := fixture()
	if _, e := c.Capture(context.Background(), Native{}, 201); e == nil {
		t.Fatal("line bound")
	}
}
func TestRunnerBoundTimeoutAndErrors(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Unix fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "backend")
	if e := os.WriteFile(script, []byte("#!/bin/sh\nwhile :; do printf '0123456789012345678901234567890123456789'; done\n"), 0700); e != nil {
		t.Fatal(e)
	}
	r, e := (ExecRunner{}).Run(context.Background(), script, nil, 1024)
	if !errors.Is(e, ErrLimit) || !r.Truncated || len(r.Bytes) > 1024 {
		t.Fatalf("%d %+v %v", len(r.Bytes), r, e)
	}
	os.WriteFile(script, []byte("#!/bin/sh\nexec /bin/sleep 10\n"), 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, e = (ExecRunner{}).Run(ctx, script, nil, 1024)
	if !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	os.WriteFile(script, []byte("#!/bin/sh\nprintf 'private-backend-secret' >&2\nexit 1\n"), 0700)
	failed, failure := (ExecRunner{}).Run(context.Background(), script, nil, 1024)
	if failure != ErrUnavailable || len(failed.Bytes) != 0 || strings.Contains(failure.Error(), "private-backend-secret") {
		t.Fatal("backend diagnostics leaked")
	}
	_, e = (ExecRunner{}).Run(context.Background(), filepath.Join(dir, "secret-path-missing"), nil, 1024)
	if e != ErrUnavailable {
		t.Fatal("raw backend error", e)
	}
}

func TestUnreviewedConfigurationNeverConnects(t *testing.T) {
	if _, err := New(Config{Executable: "/trusted/tmux", Socket: "/private/socket"}); err == nil {
		t.Fatal("unknown command-alias configuration accepted")
	}
}

func TestReviewBeforeEveryInvocation(t *testing.T) {
	for _, which := range []string{"unreviewed", "legacy_boolean", "socket_replaced", "server_replaced", "revoked", "wrong_scope", "model", "remote"} {
		t.Run(which, func(t *testing.T) {
			c, r, p := fixture()
			switch which {
			case "unreviewed":
				c.Config.ReadConfigReviewed = false
			case "legacy_boolean":
				c.Config.Review = nil
			case "socket_replaced":
				p.socket = "replacement"
			case "server_replaced":
				c.Config.Review.ServerPID = 10
				c.Config.Review.ServerStart = "old"
			case "revoked":
				c.ReviewCurrent = func() bool { return false }
			case "wrong_scope":
				c.Config.Review.Scope = "control"
			case "model":
				c.Config.Review.Reviewer.Kind = executiontarget.Model
			case "remote":
				c.Config.Review.Reviewer.Origin = executiontarget.Remote
			}
			if _, err := c.Resolve(context.Background(), "%4"); err == nil {
				t.Fatal("untrusted invocation accepted")
			}
			if len(r.calls) != 0 {
				t.Fatal("executed before review check", r.calls)
			}
		})
	}
}
func TestReviewCannotBeIssuedByModelOrRemote(t *testing.T) {
	for _, actor := range []executiontarget.Actor{{ID: "model", Kind: executiontarget.Model, Origin: executiontarget.Local}, {ID: "phone", Kind: executiontarget.Human, Origin: executiontarget.Remote}} {
		if _, err := AcceptReadConfig(Config{}, actor); err == nil {
			t.Fatal("nonlocal review accepted")
		}
	}
}

func TestWithdrawalDuringVersionProbePreventsConnection(t *testing.T) {
	c, r, _ := fixture()
	trusted := true
	c.ReviewCurrent = func() bool { return trusted }
	r.afterVersion = func() { trusted = false }
	if _, err := c.Resolve(context.Background(), "%4"); err == nil {
		t.Fatal("revocation bypassed")
	}
	if len(r.calls) != 1 || r.calls[0][0] != "-V" {
		t.Fatal("connected after review withdrawn", r.calls)
	}
}
