package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/attachment"
	"github.com/gongahkia/gator/internal/workspace"
)

// attachmentFlags accepts one explicit repository-relative path per flag.
// Separate --image and --attach flags keep CLI consent and media handling
// unambiguous without accepting arbitrary local paths.
type attachmentFlags []string

func (v *attachmentFlags) String() string {
	return strings.Join(*v, ", ")
}

func (v *attachmentFlags) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("attachment path must not be empty")
	}
	*v = append(*v, value)
	return nil
}

func loadPromptAttachments(repository string, imagePaths, documentPaths []string) ([]agent.Image, []agent.Attachment, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return nil, nil, fmt.Errorf("open repository for prompt attachments: %w", err)
	}
	inputs := make([]attachment.Input, 0, len(imagePaths)+len(documentPaths))
	for _, path := range imagePaths {
		inputs = append(inputs, attachment.Input{Path: path, Kind: attachment.ImageInput})
	}
	for _, path := range documentPaths {
		inputs = append(inputs, attachment.Input{Path: path, Kind: attachment.DocumentInput})
	}
	return attachment.LoadInputs(root, inputs)
}

func writePromptAttachmentSummary(out io.Writer, images []agent.Image, attachments []agent.Attachment) error {
	if len(images)+len(attachments) == 0 {
		return nil
	}
	parts := make([]string, 0, len(images)+len(attachments))
	for _, image := range images {
		parts = append(parts, fmt.Sprintf("%s (%s, %d bytes)", image.Name, image.MediaType, len(image.Data)))
	}
	for _, attachment := range attachments {
		parts = append(parts, fmt.Sprintf("%s (%s, %d bytes)", attachment.Name, attachment.MediaType, len(attachment.Data)))
	}
	_, err := fmt.Fprintf(out, "  attachments: %s\n", strings.Join(parts, "; "))
	return err
}
