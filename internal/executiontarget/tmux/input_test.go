package tmux

import (
	"context"
	"errors"
	"github.com/gongahkia/gator/internal/executiontarget"
	"strings"
	"testing"
)

type inputRunner struct {
	*fakeRunner
	inputs [][]string
	failAt int
	before func()
}

func (r *inputRunner) Run(ctx context.Context, bin string, args []string, limit int) (Result, error) {
	if len(args) > 5 && args[5] == "send-keys" {
		r.inputs = append(r.inputs, append([]string(nil), args...))
		if r.before != nil {
			r.before()
		}
		if len(r.inputs) == r.failAt {
			return Result{}, errors.New("private backend secret")
		}
		return Result{}, nil
	}
	return r.fakeRunner.Run(ctx, bin, args, limit)
}
func TestNarrowInputAndPartialDelivery(t *testing.T) {
	for _, kind := range []string{"text", "text_enter", "key", "interrupt"} {
		t.Run(kind, func(t *testing.T) {
			c, r, _ := fixture()
			n, err := c.Resolve(context.Background(), "%4")
			if err != nil {
				t.Fatal(err)
			}
			control := *c.Config.Review
			control.Scope = ControlTrustScope
			control.ServerPID = n.ServerPID
			control.ServerStart = n.ServerStart
			c.Config.ControlReview = &control
			runner := &inputRunner{fakeRunner: r}
			c.Runner = runner
			input := Input{Kind: kind}
			if kind == "text" || kind == "text_enter" {
				input.Text = "Enter; $(never interpolate)"
			}
			if kind == "key" {
				input.Key = "Up"
			}
			if d := c.Send(context.Background(), n, input, func() bool { return true }); d != executiontarget.DispatchAcknowledged {
				t.Fatal(d)
			}
			if len(runner.inputs) < 1 || runner.inputs[0][7] != "%4" {
				t.Fatal(runner.inputs)
			}
			if kind == "text" || kind == "text_enter" {
				if got := runner.inputs[0]; got[8] != "-l" || got[9] != "--" || got[10] != input.Text {
					t.Fatal(got)
				}
			}
			if kind == "text_enter" {
				if len(runner.inputs) != 2 || runner.inputs[1][9] != "Enter" {
					t.Fatal(runner.inputs)
				}
				runner.inputs = nil
				runner.failAt = 2
				if d := c.Send(context.Background(), n, input, func() bool { return true }); d != executiontarget.OutcomeUnknown || len(runner.inputs) != 2 {
					t.Fatal(d, runner.inputs)
				}
			}
			if kind == "interrupt" && runner.inputs[0][9] != "C-c" {
				t.Fatal(runner.inputs)
			}
			runner.inputs = nil
			c.Config.ControlReview = nil
			if d := c.Send(context.Background(), n, input, func() bool { return true }); d != executiontarget.NotDispatched || len(runner.inputs) != 0 {
				t.Fatal("read trust mutated")
			}
		})
	}
}
func TestInputValidation(t *testing.T) {
	for _, bad := range []Input{{Kind: "text", Text: "a\nb"}, {Kind: "text", Text: "a\rb"}, {Kind: "text", Text: "\x00"}, {Kind: "text", Text: "\t"}, {Kind: "text", Text: string([]byte{255})}, {Kind: "text", Text: strings.Repeat("x", 2049)}, {Kind: "key", Key: "C-c"}, {Kind: "key", Key: "run-shell"}, {Kind: "interrupt", Text: "x"}} {
		if bad.Valid() {
			t.Fatal("accepted", bad.Kind)
		}
	}
}
