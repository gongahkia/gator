// Command office-fixtures writes synthetic, account-free Office verification
// artifacts. Validate the PPTX output with scripts/office-validation.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gongahkia/gator/internal/presentation"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: office-fixtures output-directory")
		os.Exit(2)
	}
	if err := generate(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	for _, theme := range []string{"professional", "minimal", "report"} {
		deck, _, err := presentation.RenderPPTX(presentation.Spec{
			Title: "October service review", Theme: theme,
			Slides: []presentation.Slide{
				{Title: "Results & decisions", Body: []string{"Availability improved to 99.7%.", "One priority-one ticket remains open.", "Next: complete the regional rollout."}, Transition: "fade", Notes: "Keep the original facts visible.\nMention the unresolved ticket."},
				{Title: "Regional rollout", Body: []string{"Twelve regions are live.", "The final onboarding review is due next week."}, Transition: "push"},
				{Title: "多语言 / العربية", Body: []string{"客户服务 — 质量与下一步", "مراجعة الخدمة والخطوات التالية"}, Transition: "wipe", Notes: "Multilingual fixture; native shaping and font fallback still require visual checks."},
			},
		})
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(directory, theme+"-new.pptx"), deck, 0o600); err != nil {
			return err
		}
		edited, _, err := presentation.EditPPTX(deck, []presentation.Edit{
			{Operation: "set_title", Slide: 0, Title: "Updated results & decisions"},
			{Operation: "set_body", Slide: 0, Body: []string{"Availability remains 99.7%.", "The final ticket is resolved."}},
			{Operation: "set_notes", Slide: 0, Notes: "Revised speaker notes."},
			{Operation: "add_slide", Title: "Follow-up", Body: []string{"Review the completed rollout."}, Notes: "New notes on the added slide.", Transition: "fade"},
			{Operation: "move_slide", Slide: 3, To: 0},
			{Operation: "delete_slide", Slide: 2},
		})
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(directory, theme+"-edited.pptx"), edited, 0o600); err != nil {
			return err
		}
	}
	return nil
}
