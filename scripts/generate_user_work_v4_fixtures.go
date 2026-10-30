package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gongahkia/gator/internal/document"
	"github.com/gongahkia/gator/internal/presentation"
)

// This generator creates small synthetic Office/PDF source files for the
// user-work-v4 evaluation corpus. It is not used while evaluating Gator.
func main() {
	deckOnly := flag.Bool("deck-only", false, "regenerate only the presentation fixture")
	flag.Parse()
	root := filepath.Join("internal", "eval", "testdata", "user-work-v4", "sources")
	writePDF := func(name string, spec document.Spec) {
		contents, _, err := document.RenderPDF(spec)
		if err != nil {
			panic(err)
		}
		write(root, name, contents)
	}
	if !*deckOnly {
		writePDF("qbr-pdf-to-memo/quarterly-results.pdf", document.Spec{Title: "Harbor Q3 account review", Blocks: []document.Block{
			{Kind: "heading", Level: 1, Text: "Quarterly results"},
			{Kind: "paragraph", Text: "Quarter: July through September 2026. Contracted seats: 120. Active seats at quarter end: 96, versus 84 at prior quarter end."},
			{Kind: "paragraph", Text: "Support tickets opened: 18; 15 closed by quarter end; 3 remain open. Of those three, one export-delay ticket is priority one."},
			{Kind: "paragraph", Text: "Annual renewal date: 15 December 2026. Expansion discussions have not been approved by the customer."},
		}})
		writePDF("receipt-expense-sheet/train-receipt.pdf", document.Spec{Title: "Rail receipt", Blocks: []document.Block{
			{Kind: "paragraph", Text: "Receipt R-801. 14 October 2026. Rail fare USD 48.60. Paid by employee Mei. Project P-19. Reimbursable travel."},
		}})
		writePDF("receipt-expense-sheet/hotel-receipt.pdf", document.Spec{Title: "Hotel receipt", Blocks: []document.Block{
			{Kind: "paragraph", Text: "Receipt R-802. 15 October 2026. Hotel room USD 210.00. Paid by employee Mei. Project P-19. Manager approval on file."},
		}})
		writePDF("receipt-expense-sheet/meal-receipt.pdf", document.Spec{Title: "Meal receipt", Blocks: []document.Block{
			{Kind: "paragraph", Text: "Receipt R-803. 15 October 2026. Team meal USD 36.40. Paid by employee Mei. Project P-19. Alcohol USD 8.00 is included in this total."},
		}})
	}
	deck, _, err := presentation.RenderPPTX(presentation.Spec{Title: "Atlas monthly review", Slides: []presentation.Slide{
		{Title: "Atlas monthly review", Body: []string{"September 2026 draft", "Prepared for operations leadership"}},
		{Title: "Service health", Body: []string{"Availability: 99.2%", "Open priority-one tickets: 2"}},
		{Title: "Next actions", Body: []string{"Support to review export alerts"}},
	}})
	if err != nil {
		panic(err)
	}
	write(root, "deck-update/monthly-review.pptx", deck)
	if *deckOnly {
		fmt.Println("generated PPTX fixture")
	} else {
		fmt.Println("generated four PDF fixtures and one PPTX fixture")
	}
}

func write(root, relative string, contents []byte) {
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		panic(err)
	}
}
