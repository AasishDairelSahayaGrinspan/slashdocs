package internal

import (
	"embed"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// guidelinesFS holds the authoring guidance shipped with the binary. The core
// contract and every category playbook live as markdown so an agent can read
// them straight from `aidocs guidelines [category]` before writing a document.
//
//go:embed guidelines/*.md
var guidelinesFS embed.FS

// categories maps the `aidocs guidelines <category>` arg to its embedded file.
var categories = map[string]string{
	"overview":      "communicating-with-html.md",
	"deck":          "deck.md",
	"pr-review":     "pr-and-code-review.md",
	"comparison":    "comparison-and-options.md",
	"report":        "reports-and-rca.md",
	"explainer":     "explainer-and-diagrams.md",
	"writing-voice": "writing-voice.md",
	"theme":         "razorpay-theme.md",
}

func readGuideline(name string) (string, error) {
	b, err := guidelinesFS.ReadFile("guidelines/" + name)
	return string(b), err
}

func categoryList() string {
	keys := make([]string, 0, len(categories))
	for k := range categories {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func guidelinesCmd(_ *globals, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:          "guidelines [category]",
		Short:        "Print HTML authoring guidelines (core, or a category playbook)",
		Long:         "Run with no args for the core contract. Pass a category for its playbook.\nCategories: " + categoryList(),
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				core, err := readGuideline("core.md")
				if err != nil {
					return err
				}
				fmt.Fprint(out, core)
				return nil
			}
			file, ok := categories[args[0]]
			if !ok {
				return fmt.Errorf("unknown category %q; choose one of: %s", args[0], categoryList())
			}
			body, err := readGuideline(file)
			if err != nil {
				return err
			}
			fmt.Fprint(out, body)
			return nil
		},
	}
}
