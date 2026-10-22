package filelock

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProcessLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lease")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := Lock(file, Exclusive); err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("metadata"); err != nil {
		t.Fatal(err)
	}
	probe := func(want string) {
		t.Helper()
		command := exec.Command(os.Args[0], "-test.run=^TestLeaseChild$")
		command.Env = append(os.Environ(), "GATOR_TEST_LEASE="+path, "GATOR_TEST_LEASE_WANT="+want)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("lease child: %v\n%s", err, out)
		}
	}
	probe("busy")
	if err := Unlock(file); err != nil {
		t.Fatal(err)
	}
	probe("available")
	// The child's exit releases its exclusive lock without an explicit unlock.
	if err := Lock(file, Exclusive|Nonblocking); err != nil {
		t.Fatal(err)
	}
	if err := Unlock(file); err != nil {
		t.Fatal(err)
	}
	if err := Lock(file, Shared); err != nil {
		t.Fatal(err)
	}
	other, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := Lock(other, Shared|Nonblocking); err != nil {
		t.Fatal(err)
	}
	if err := Unlock(other); err != nil {
		t.Fatal(err)
	}
	probe("busy")
}

func TestLeaseChild(t *testing.T) {
	path := os.Getenv("GATOR_TEST_LEASE")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "metadata" {
		t.Fatalf("lease obstructed metadata read: %q, %v", data, err)
	}
	err = Lock(file, Exclusive|Nonblocking)
	if os.Getenv("GATOR_TEST_LEASE_WANT") == "busy" {
		if !errors.Is(err, ErrWouldBlock) {
			t.Fatalf("expected busy lease: %v", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
}

func TestRootLockRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "target"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(directory, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if file, err := OpenRootFile(root, "link", os.O_RDWR, 0600); err == nil {
		file.Close()
		t.Fatal("accepted symlink lock")
	}
}
