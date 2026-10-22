package attachment

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

var uploadID = regexp.MustCompile(`^[a-f0-9]{32}/[^/\\\x00-\x1f]{1,200}$`)

func ValidUploadID(id string) bool {
	return uploadID.MatchString(id) && filepath.Base(id) != "." && filepath.Base(id) != ".."
}

// LoadUploads reads bounded copies under the host-selected upload root.
// Renderer identities never grant access to arbitrary absolute paths.
func LoadUploads(directory string, ids []string) ([]agent.Image, []agent.Attachment, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	if strings.TrimSpace(directory) == "" {
		return nil, nil, errors.New("desktop attachments are unavailable")
	}
	root, err := workspace.Open(directory)
	if err != nil {
		return nil, nil, err
	}
	inputs := make([]Input, 0, len(ids))
	for _, id := range ids {
		if !ValidUploadID(id) {
			return nil, nil, errors.New("invalid desktop attachment identity")
		}
		kind := DocumentInput
		if IsImage(id) {
			kind = ImageInput
		}
		inputs = append(inputs, Input{Path: id, Kind: kind})
	}
	images, documents, err := LoadInputs(root, inputs)
	for i := range images {
		images[i].Name = filepath.Base(images[i].Name)
	}
	for i := range documents {
		documents[i].Name = filepath.Base(documents[i].Name)
	}
	return images, documents, err
}
