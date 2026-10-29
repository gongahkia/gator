package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gongahkia/gator/internal/presentation"
)

func layoutFixtures(directory string) error {
	body := make([]string, 16)
	for index := range body {
		body[index] = fmt.Sprintf("Review item %02d: confirm the regional delivery date, assigned owner, unresolved risks and next action.", index+1)
	}
	contents, _, err := presentation.RenderPPTX(presentation.Spec{Title: "Dense review fixture", Slides: []presentation.Slide{{Title: "Regional delivery review: ownership, remaining risks, customer commitments and next actions", Body: body}}})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "dense-review.pptx"), contents, 0600)
}
