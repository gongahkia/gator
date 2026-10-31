package external

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/companionaccess"
	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/executiontarget/tmux"
)

type nativeFixture struct {
	mu      sync.Mutex
	n       tmux.Native
	capture func()
	reads   int
}

func (f *nativeFixture) Resolve(context.Context, string) (tmux.Native, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.n, nil
}
func (f *nativeFixture) Capture(context.Context, tmux.Native, int) (tmux.Capture, error) {
	if f.capture != nil {
		f.capture()
	}
	return tmux.Capture{Text: "<script>private 猫</script>", CapturedAt: time.Now()}, nil
}
func setup(t *testing.T) (Store, *nativeFixture, companionaccess.Reference, executiontarget.Actor, Entry) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	token := filepath.Join(dir, "token")
	os.WriteFile(token, []byte(strings.Repeat("a", 64)+"\n"), 0600)
	authority := companionaccess.Reference{TokenFile: token, Device: "owner"}
	id, e := authority.Actor()
	if e != nil {
		t.Fatal(e)
	}
	actor := executiontarget.Actor{ID: id, Kind: executiontarget.Human, Origin: executiontarget.Remote}
	s, e := Open(filepath.Join(dir, "state"))
	if e != nil {
		t.Fatal(e)
	}
	f := &nativeFixture{n: tmux.Native{ServerPID: 10, ServerTime: "100", ServerStart: "os-start", SocketEvidence: "dev-inode-ctime", Session: "$1", SessionCreated: "101", Pane: "%4", PanePID: 11, PaneStart: "pane-start", Height: 24}}
	s.OpenReader = func(tmux.Config) (NativeReader, error) { return f, nil }
	s.Fingerprint = func(string) (string, error) { return "digest", nil }
	entry, e := s.Enroll(ctx, tmux.Config{Executable: "/trusted/tmux", Socket: "/private/socket", ReadConfigReviewed: true, Review: &tmux.Review{SocketEvidence: f.n.SocketEvidence, ExecutableDigest: "digest", Reviewer: executiontarget.Actor{ID: "local", Kind: executiontarget.Human, Origin: executiontarget.Local}, ReviewedAt: time.Now().Add(-time.Second), Scope: tmux.ReadTrustScope}}, "%4", "My pane")
	if e != nil {
		t.Fatal(e)
	}
	return s, f, authority, actor, entry
}
func TestEnrollmentAndSeparatedInspectGrant(t *testing.T) {
	s, f, a, actor, e := setup(t)
	ctx := context.Background()
	b := e.Target.Binding
	reads := f.reads
	if _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
		t.Fatal("enrollment implied permission")
	}
	if f.reads != reads {
		t.Fatal("unauthorized backend read")
	}
	if err := s.Grant(ctx, b.ID, a, 901); err == nil {
		t.Fatal("extended expiry")
	}
	if err := s.Grant(ctx, b.ID, a, 60); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err != nil || !strings.Contains(v.Text, "private") {
		t.Fatal(v, err)
	}
	for _, bad := range []executiontarget.Actor{{ID: actor.ID, Kind: executiontarget.Model, Origin: executiontarget.Remote}, {ID: "inspection-" + strings.Repeat("b", 64), Kind: executiontarget.Human, Origin: executiontarget.Remote}} {
		if _, err := s.Read(ctx, bad, b.ID, b.Generation, 100, true); err == nil {
			t.Fatal("wrong actor")
		}
	}
	if _, err := s.Read(ctx, actor, b.ID, strings.Repeat("c", 32), 100, true); err == nil {
		t.Fatal("wrong generation")
	}
	s.transaction(true, func(c *catalog) error { c.Entries[0].Grants[0].Grant.Operation = executiontarget.Cancel; return nil })
	if _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
		t.Fatal("wrong operation")
	}
}
func TestStaleEnrollmentNeverRevives(t *testing.T) {
	s, f, a, actor, e := setup(t)
	ctx := context.Background()
	b := e.Target.Binding
	s.Grant(ctx, b.ID, a, 60)
	original := f.n
	f.n.ServerStart = "new-server"
	if _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
		t.Fatal("server reused identity")
	}
	f.n = original
	if _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
		t.Fatal("revived")
	}
	if err := s.Grant(ctx, b.ID, a, 60); err == nil {
		t.Fatal("grant revived tombstone")
	}
}
func TestRevocationAndExpiryBeforeResponse(t *testing.T) {
	for _, what := range []string{"enrollment", "grant", "device", "expiry"} {
		t.Run(what, func(t *testing.T) {
			s, f, a, actor, e := setup(t)
			ctx := context.Background()
			b := e.Target.Binding
			if err := s.Grant(ctx, b.ID, a, 60); err != nil {
				t.Fatal(err)
			}
			f.capture = func() {
				switch what {
				case "enrollment":
					s.Revoke(b.ID)
				case "grant":
					s.Grant(ctx, b.ID, a, 0)
				case "device":
					os.WriteFile(a.TokenFile, []byte(strings.Repeat("b", 64)+"\n"), 0600)
				case "expiry":
					s.transaction(true, func(c *catalog) error {
						c.Entries[0].Grants[0].Grant.ExpiresAt = time.Now().Add(-time.Second)
						return nil
					})
				}
			}
			if _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
				t.Fatal("sensitive snapshot returned after revoke")
			}
		})
	}
}
func TestConcurrentCaptureAndEnrollmentRemoval(t *testing.T) {
	s, f, a, actor, e := setup(t)
	ctx := context.Background()
	b := e.Target.Binding
	s.Grant(ctx, b.ID, a, 60)
	entered, release := make(chan struct{}), make(chan struct{})
	f.capture = func() { close(entered); <-release }
	done := make(chan error, 1)
	go func() { _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); done <- err }()
	<-entered
	if err := s.Revoke(b.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("queued snapshot leaked")
	}
	entries, err := s.LocalEntries()
	if err != nil || !entries[0].Invalid || len(entries[0].Grants) != 0 {
		t.Fatal(entries, err)
	}
}

func TestPersistenceExactBackendAndGrantBoundaries(t *testing.T) {
	s, f, a, actor, e := setup(t)
	ctx := context.Background()
	b := e.Target.Binding
	if err := s.Grant(ctx, b.ID, a, 60); err != nil {
		t.Fatal(err)
	}
	// Reopening the private catalog retains authority, never substitutes native state.
	reopened, err := Open(filepath.Dir(filepath.Dir(s.root.Path())))
	if err != nil {
		t.Fatal(err)
	}
	reopened.OpenReader = s.OpenReader
	reopened.Fingerprint = s.Fingerprint
	if _, err = reopened.Read(ctx, actor, b.ID, b.Generation, 100, true); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []executiontarget.Operation{executiontarget.Cancel, executiontarget.Steer, executiontarget.TextInput, executiontarget.KeyInput, executiontarget.Interrupt} {
		entry, _ := s.entry(b.ID)
		g := entry.Grants[0].Grant
		if executiontarget.Check(time.Now(), actor, b, entry.Target, operation, g, &entry.Enrollment) == nil {
			t.Fatal("Inspect authorized", operation)
		}
	}
	// A Work grant cannot become external Inspect authority even for the same actor.
	s.transaction(true, func(c *catalog) error { c.Entries[0].Grants[0].Grant.Target.Backend = executiontarget.Work; return nil })
	if _, err = s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
		t.Fatal("Work grant authorized tmux")
	}
	f.n.PaneStart = "replaced"
	if _, err = reopened.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
		t.Fatal("retained metadata invented liveness")
	}
}
func TestBackendReplacementAndNoSilentReenrollment(t *testing.T) {
	for _, which := range []string{"socket", "executable", "pane", "membership", "process", "server"} {
		t.Run(which, func(t *testing.T) {
			s, f, a, actor, e := setup(t)
			ctx := context.Background()
			b := e.Target.Binding
			s.Grant(ctx, b.ID, a, 60)
			switch which {
			case "socket":
				f.n.SocketEvidence = "other-backend"
			case "executable":
				s.Fingerprint = func(string) (string, error) { return "replaced", nil }
			case "pane":
				f.n.Pane = "%99"
			case "membership":
				f.n.Session = "$2"
			case "process":
				f.n.PaneStart = "respawn"
			case "server":
				f.n.ServerTime = "new"
			}
			if _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
				t.Fatal("stale native binding accepted")
			}
			entries, _ := s.LocalEntries()
			if !entries[0].Invalid || len(entries[0].Grants) != 0 {
				t.Fatal("failure did not invalidate enrollment")
			}
		})
	}
}
func TestRevocationDuringFinalNativeRevalidation(t *testing.T) {
	s, _, a, actor, e := setup(t)
	ctx := context.Background()
	b := e.Target.Binding
	s.Grant(ctx, b.ID, a, 60)
	// Final authorization must follow all potentially blocking backend probes.
	base := s.OpenReader
	calls := 0
	s.OpenReader = func(c tmux.Config) (NativeReader, error) {
		r, err := base(c)
		calls++
		if calls == 1 {
			return r, err
		}
		return finalProbe{NativeReader: r, revoke: func() { s.Grant(ctx, b.ID, a, 0) }}, err
	}
	if _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
		t.Fatal("released after grant was withdrawn")
	}
}

type finalProbe struct {
	NativeReader
	revoke func()
}

func (f finalProbe) Resolve(ctx context.Context, pane string) (tmux.Native, error) {
	f.revoke()
	return f.NativeReader.Resolve(ctx, pane)
}

func TestCatalogRejectsPublicAndLinkedStorage(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Unix permissions")
	}
	s, _, _, _, _ := setup(t)
	path := filepath.Join(s.root.Path(), "catalog.json")
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LocalEntries(); err == nil {
		t.Fatal("public catalog accepted")
	}
	os.Chmod(path, 0600)
	os.Remove(path)
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LocalEntries(); err == nil {
		t.Fatal("linked catalog accepted")
	}
}

func TestReviewAndInspectIndependent(t *testing.T) {
	s, f, a, actor, e := setup(t)
	ctx := context.Background()
	b := e.Target.Binding
	if err := s.Grant(ctx, b.ID, a, 60); err != nil {
		t.Fatal(err)
	}
	before := f.reads
	s.transaction(true, func(c *catalog) error { c.Entries[0].Config.Review = nil; return nil })
	if _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
		t.Fatal("grant implied review")
	}
	if f.reads != before {
		t.Fatal("unreviewed target invoked backend")
	}
	if _, err := s.verify(ctx, func() Entry { v, _ := s.entry(b.ID); return v }()); err == nil {
		t.Fatal("status/grant path bypassed review")
	}
}
func TestReviewRevocationDuringCapture(t *testing.T) {
	s, f, a, actor, e := setup(t)
	ctx := context.Background()
	b := e.Target.Binding
	s.Grant(ctx, b.ID, a, 60)
	f.capture = func() {
		if err := s.Revoke(b.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
		t.Fatal("released after review revoked")
	}
	retained, _ := s.entry(b.ID)
	if retained.Config.Review != nil || len(retained.Grants) != 0 {
		t.Fatal("review/grants retained")
	}
}
func TestUnreviewEntireBackendWithoutInvocation(t *testing.T) {
	s, f, a, actor, first := setup(t)
	ctx := context.Background()
	second, err := s.Enroll(ctx, first.Config, "%4", "Other enrollment")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []Entry{first, second} {
		if err := s.Grant(ctx, entry.Target.Binding.ID, a, 60); err != nil {
			t.Fatal(err)
		}
	}
	before := f.reads
	if err := s.Unreview(first.Target.Binding.ID); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []Entry{first, second} {
		b := entry.Target.Binding
		if _, err := s.Read(ctx, actor, b.ID, b.Generation, 100, true); err == nil {
			t.Fatal("server-wide withdrawal bypassed")
		}
	}
	if f.reads != before {
		t.Fatal("unreview or subsequent request invoked backend")
	}
}
