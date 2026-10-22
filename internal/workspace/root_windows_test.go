package workspace

import "testing"

func TestWindowsRejectsDeviceDriveAndAlternateStreamPaths(t *testing.T) {
	root, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{`C:\outside.txt`, `C:relative.txt`, `\rooted.txt`, `\\server\share\file`, `note.txt:secret`, `NUL`, `NUL.txt`, `CON`, `COM1`, `..\outside`} {
		if _, err := root.ResolveFile(path); err == nil {
			t.Errorf("accepted non-local Windows path %q", path)
		}
	}
}
