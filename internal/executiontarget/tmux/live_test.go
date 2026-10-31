package tmux

import (
	"context"
	"github.com/gongahkia/gator/internal/executiontarget"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Explicit opt-in. Every mutating fixture command addresses only the fresh
// test-created private socket. Production exposes none of these commands.
func TestLiveIsolatedTmux(t *testing.T) {
	if os.Getenv("GATOR_TMUX_LIVE") != "1" {
		t.Skip("opt in with GATOR_TMUX_LIVE=1; isolated test-created server only")
	}
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "gt3-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "mux.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, bin, append([]string{"-f", os.DevNull, "-S", socket}, args...)...)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "TERM=dumb", "LC_ALL=C"}
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("fixture %s: %v %s", args[0], e, out)
		}
		return strings.TrimSpace(string(out))
	}
	missing, _ := New(Config{Executable: bin, Socket: socket, ReadConfigReviewed: true})
	if missing != nil {
		t.Fatal("missing server accepted")
	}
	if _, err := os.Lstat(socket); !os.IsNotExist(err) {
		t.Fatal("inspection created server socket")
	}
	fixture("new-session", "-d", "-s", "test", "cat")
	defer func() { cmd := exec.Command(bin, "-N", "-S", socket, "kill-server"); _ = cmd.Run() }()
	config, err := AcceptReadConfig(Config{Executable: bin, Socket: socket}, executiontarget.Actor{ID: "fixture-owner", Kind: executiontarget.Human, Origin: executiontarget.Local})
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := c.Candidates(ctx)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates: %+v %v", candidates, err)
	}
	native := candidates[0]
	fixture("send-keys", "-t", native.Pane, "-l", "T3 exact pane <script>untrusted</script>")
	for i := 0; i < 50; i++ {
		got, e := c.Capture(ctx, native, 20)
		if e == nil && strings.Contains(got.Text, "T3 exact pane") {
			break
		}
		if i == 49 {
			t.Fatalf("capture: %q %v", got.Text, e)
		}
		time.Sleep(10 * time.Millisecond)
	}
	fixture("split-window", "-d", "-t", native.Pane, "cat")
	fixture("select-pane", "-t", "%1")
	fixture("rename-window", "renamed")
	got, err := c.Capture(ctx, native, 20)
	if err != nil || !strings.Contains(got.Text, "T3 exact pane") {
		t.Fatalf("active pane redirected: %q %v", got.Text, err)
	}

	// Deliberately demonstrate that intended read argv is not proof of behavior.
	// These aliases and harmless option changes affect ONLY this disposable server.
	fixture("set-option", "-g", "@alias_sentinel", "no")
	fixture("set-option", "-s", "command-alias[100]", "capture-pane=set-option -g @alias_sentinel yes ; capture-pane")
	if _, err := c.Capture(ctx, native, 20); err != nil {
		t.Fatal(err)
	}
	if fixture("show-option", "-gv", "@alias_sentinel") != "yes" {
		t.Fatal("canonical alias did not expand")
	}
	fixture("set-option", "-su", "command-alias[100]")
	fixture("set-option", "-s", "command-alias[100]", "if-shell=set-option -g @alias_sentinel bootstrap ; if-shell")
	if _, err := c.Resolve(ctx, native.Pane); err != nil {
		t.Fatal(err)
	}
	if fixture("show-option", "-gv", "@alias_sentinel") != "bootstrap" {
		t.Fatal("bootstrap alias did not expand")
	}
	// An explicitly withdrawn review prevents even that bootstrap alias running.
	c.ReviewCurrent = func() bool { return false }
	fixture("set-option", "-g", "@alias_sentinel", "withdrawn")
	if _, err := c.Resolve(ctx, native.Pane); err == nil {
		t.Fatal("withdrawn trust accepted")
	}
	if fixture("show-option", "-gv", "@alias_sentinel") != "withdrawn" {
		t.Fatal("unreviewed alias executed")
	}
	c.ReviewCurrent = nil
	fixture("set-option", "-su", "command-alias[100]")
	fixture("set-option", "-s", "command-alias[100]", "capture-pane=invalid-gator-command")
	if _, err := c.Capture(ctx, native, 20); err == nil {
		t.Fatal("malformed alias accepted")
	}
	fixture("set-option", "-su", "command-alias[100]")
	fixture("set-option", "-g", "@gator_hook_called", "no")
	fixture("set-hook", "-g", "after-capture-pane", "set-option -g @gator_hook_called yes")
	if _, err := c.Capture(ctx, native, 20); err == nil {
		t.Fatal("capture with executable hook allowed")
	}
	if fixture("show-option", "-gv", "@gator_hook_called") != "no" {
		t.Fatal("read fired a mutation hook")
	}
	fixture("set-hook", "-gu", "after-capture-pane")
	fixture("set-hook", "-g", "command-error", "set-option -g @gator_hook_called yes")
	fixture("kill-pane", "-t", native.Pane)
	if _, err := c.Capture(ctx, native, 20); err == nil {
		t.Fatal("missing pane substituted")
	}
	if fixture("show-option", "-gv", "@gator_hook_called") != "no" {
		t.Fatal("missing-pane read fired error hook")
	}
}

func TestLiveFiniteKeyEncoding(t *testing.T) {
	if os.Getenv("GATOR_TMUX_LIVE") != "1" {
		t.Skip("isolated native key fixture opt-in")
	}
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "gt4-key-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "keys.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, bin, append([]string{"-f", os.DevNull, "-S", socket}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatal(err, string(out))
		}
		return strings.TrimSpace(string(out))
	}
	fixture("new-session", "-d", "-s", "keys", "sh", "-c", "stty raw -echo; od -An -tx1 -v")
	defer func() { exec.Command(bin, "-N", "-S", socket, "kill-server").Run() }()
	config, err := AcceptReadConfig(Config{Executable: bin, Socket: socket}, executiontarget.Actor{ID: "fixture", Kind: executiontarget.Human, Origin: executiontarget.Local})
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	panes, err := c.Candidates(ctx)
	if err != nil || len(panes) != 1 {
		t.Fatal(err)
	}
	n := panes[0]
	review := *config.Review
	review.ServerPID = n.ServerPID
	review.ServerStart = n.ServerStart
	review.Scope = ControlTrustScope
	c.Config.ControlReview = &review
	fixture("set-option", "-g", "@t4_alias_seen", "no")
	fixture("set-option", "-s", "command-alias[110]", "send-keys=set-option -g @t4_alias_seen yes ; send-keys")
	for _, key := range []string{"Enter", "Escape", "Tab", "Backspace", "Up", "Down", "Left", "Right"} {
		if d := c.Send(ctx, n, Input{Kind: "key", Key: key}, func() bool { return true }); d != executiontarget.DispatchAcknowledged {
			t.Fatal(key, d)
		}
	}
	if fixture("show-option", "-gv", "@t4_alias_seen") != "yes" {
		t.Fatal("control alias fixture did not execute")
	}
	fixture("set-option", "-su", "command-alias[110]")
	want := "0d 1b 09 7f 1b 5b 41 1b 5b 42 1b 5b 44 1b 5b 43"
	for i := 0; i < 50; i++ {
		got, err := c.Capture(ctx, n, 20)
		if err == nil && strings.Contains(strings.Join(strings.Fields(got.Text), " "), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("native key bytes not observed", fixture("capture-pane", "-p", "-t", n.Pane))
}
