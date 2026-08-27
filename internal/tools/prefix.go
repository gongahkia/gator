package tools

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"
)

var rejectedCommandLaunchers = map[string]struct{}{
	"bash": {}, "sh": {}, "dash": {}, "zsh": {}, "ksh": {}, "csh": {}, "tcsh": {}, "fish": {},
	"cmd": {}, "cmd.exe": {}, "powershell": {}, "powershell.exe": {}, "pwsh": {}, "pwsh.exe": {},
	"env": {}, "xargs": {}, "nice": {}, "nohup": {}, "stdbuf": {}, "timeout": {},
	"sudo": {}, "doas": {}, "su": {}, "script": {},
}

// ValidateCommandPrefix accepts one anchored literal-token prefix. It rejects
// empty patterns, glob or shell-text syntax, and wrapper launchers that would
// turn a prefix into an unbounded command string.
func ValidateCommandPrefix(pattern []string) error {
	if len(pattern) == 0 || strings.TrimSpace(pattern[0]) == "" {
		return errors.New("command prefix must contain a program token")
	}
	if len(pattern) > 32 {
		return errors.New("command prefix has too many tokens")
	}
	for index, token := range pattern {
		if token == "" || strings.ContainsAny(token, "\x00\r\n") {
			return fmt.Errorf("command prefix token %d is empty or contains a newline", index+1)
		}
		if err := rejectShellOrGlobToken(token); err != nil {
			return err
		}
	}
	program := path.Base(pattern[0])
	if _, rejected := rejectedCommandLaunchers[strings.ToLower(program)]; rejected {
		return fmt.Errorf("command prefix must not begin with launcher %q", program)
	}
	return nil
}

// PrefixAllows reports whether argv starts with every literal token in pattern.
// Matching is token-boundary exact equality; it never expands globs or shell.
func PrefixAllows(prefixes [][]string, argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	for _, pattern := range prefixes {
		if len(pattern) == 0 || len(pattern) > len(argv) {
			continue
		}
		matched := true
		for index, token := range pattern {
			if argv[index] != token {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func rejectShellOrGlobToken(token string) error {
	if strings.ContainsAny(token, `*?[]{}~$&|;<>()!\`+"`") {
		return fmt.Errorf("command prefix token %q must be a literal argv token", token)
	}
	for _, character := range token {
		if unicode.IsSpace(character) {
			return fmt.Errorf("command prefix token %q must not contain whitespace", token)
		}
	}
	return nil
}
