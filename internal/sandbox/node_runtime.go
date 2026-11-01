package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var nvmVersion = regexp.MustCompile(`\Av[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?\z`)

// commandRuntimeRoots exposes only the selected host-installed NVM Node
// runtime code (node plus npm), read-only. Granting the whole home/PATH directory would disclose
// unrelated files. Other nonstandard runtimes still need explicit roots.
func commandRuntimeRoots(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	name := filepath.Base(argv[0])
	if name != "node" && name != "npm" && name != "npx" {
		return nil
	}
	executable, err := exec.LookPath(argv[0])
	if err != nil {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	root := nvmRuntimeRoot(executable, home)
	if root == "" {
		return nil
	}
	return uniqueExistingRoots([]string{filepath.Join(root, "bin"), filepath.Join(root, "lib", "node_modules", "npm")})
}

func nvmRuntimeRoot(executable, home string) string {
	home, err := filepath.EvalSymlinks(home)
	if err != nil {
		return ""
	}
	native, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return ""
	}
	base := filepath.Join(home, ".nvm", "versions", "node")
	relative, err := filepath.Rel(base, native)
	if err != nil {
		return ""
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) < 3 || !nvmVersion.MatchString(parts[0]) {
		return ""
	}
	root := filepath.Join(base, parts[0])
	allowed := native == filepath.Join(root, "bin", "node") || native == filepath.Join(root, "lib", "node_modules", "npm", "bin", "npm-cli.js") || native == filepath.Join(root, "lib", "node_modules", "npm", "bin", "npx-cli.js")
	if !allowed {
		return ""
	}
	node, err := filepath.EvalSymlinks(filepath.Join(root, "bin", "node"))
	if err != nil || node != filepath.Join(root, "bin", "node") {
		return ""
	}
	if info, err := os.Stat(node); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return root
}
