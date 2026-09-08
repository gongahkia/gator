package artifact

import (
	"errors"
	"fmt"
	"path/filepath"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/workspace"
)

// WriteText stages one portable UTF-8 artifact under an isolated output root.
// It enforces the contract's per-artifact limit at every write; the complete
// bundle and total-size limit are checked by Inspect before completion.
func WriteText(root workspace.Root, contract Contract, path, content string) error {
	contract = contract.Normalize()
	if err := contract.Validate(); err != nil {
		return fmt.Errorf("validate artifact contract: %w", err)
	}
	if err := ValidateOutputPath(path); err != nil {
		return err
	}
	if !utf8.ValidString(content) {
		return errors.New("artifact content must be UTF-8 text")
	}
	for _, character := range content {
		if character == 0 {
			return errors.New("artifact content must not contain NUL bytes")
		}
	}
	return root.WriteRegularFileAtomic(filepath.FromSlash(path), []byte(content), contract.MaxArtifactBytes)
}
