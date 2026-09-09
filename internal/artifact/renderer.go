package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
)

// NewRendererEvidence hashes trusted semantic input after a renderer has
// successfully published the corresponding artifact bytes.
func NewRendererEvidence(path, renderer string, spec any, template, contents []byte) (RendererEvidence, error) {
	payload, err := json.Marshal(spec)
	if err != nil {
		return RendererEvidence{}, err
	}
	specDigest := sha256.Sum256(payload)
	artifactDigest := sha256.Sum256(contents)
	evidence := RendererEvidence{
		Path:           filepath.ToSlash(path),
		Format:         strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		Renderer:       renderer,
		ArtifactSHA256: hex.EncodeToString(artifactDigest[:]),
		SpecSHA256:     hex.EncodeToString(specDigest[:]),
	}
	if template != nil {
		templateDigest := sha256.Sum256(template)
		evidence.TemplateSHA256 = hex.EncodeToString(templateDigest[:])
	}
	if err := evidence.Validate(); err != nil {
		return RendererEvidence{}, err
	}
	return evidence, nil
}
