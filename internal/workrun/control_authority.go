package workrun

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/gongahkia/gator/internal/companionaccess"
)

// Preserve T2 JSON and its private method while sharing registry validation.
// External targets depend on companionaccess, never Work execution.
type CompanionAuthority companionaccess.Reference

func (a CompanionAuthority) actor() (string, error) { return companionaccess.Reference(a).Actor() }
func hashText(s string) string                      { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
