package workrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/workspace"
)

// CompanionAuthority references the EXISTING companion registry. Only the
// trusted local grant workflow may choose these paths; they never come from HTTP.
// This is an enrollment-generation check, not another credential or device store.
type CompanionAuthority struct {
	TokenFile string `json:"token_file"`
	Device    string `json:"device"`
}

func hashText(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func privateAuthorityFile(path string, max int64) ([]byte, error) {
	if !filepath.IsAbs(path) || privatefs.Check(filepath.Dir(path)) != nil || privatefs.Check(path) != nil {
		return nil, errors.New("companion authority unavailable")
	}
	root, err := workspace.Open(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	return root.ReadRegularFile(filepath.Base(path), max)
}

func (a CompanionAuthority) actor() (string, error) {
	bytes, err := privateAuthorityFile(a.TokenFile, 65)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(bytes))
	if len(bytes) != 65 || !controlDigest.MatchString(token) {
		return "", errors.New("invalid companion authority")
	}
	generation := hashText(token)
	identity := "owner:" + generation
	if a.Device != "owner" {
		if !controlToken.MatchString(a.Device) {
			return "", errors.New("invalid enrolled device")
		}
		bytes, err = privateAuthorityFile(filepath.Join(a.TokenFile+".devices", a.Device+".json"), 4096)
		if err != nil {
			return "", err
		}
		var device struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
			Owner   string `json:"owner_hash"`
			Secret  string `json:"secret_hash"`
			Name    string `json:"name"`
			Created string `json:"created_at"`
		}
		if json.Unmarshal(bytes, &device) != nil || device.Version != 1 || device.ID != a.Device || device.Owner != generation || !controlDigest.MatchString(device.Secret) || strings.TrimSpace(device.Name) == "" || utf8.RuneCountInString(device.Name) > 80 || !utf8.ValidString(device.Name) || strings.IndexFunc(device.Name, unicode.IsControl) >= 0 {
			return "", errors.New("device revoked or changed")
		}
		if _, err := time.Parse(time.RFC3339Nano, device.Created); err != nil {
			return "", errors.New("invalid device enrollment")
		}
		identity = "device:" + device.ID + ":" + generation + ":" + device.Secret
	}
	return "inspection-" + hashText(identity), nil
}
