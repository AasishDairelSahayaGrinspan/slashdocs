package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
)

func skillsCmd(g *globals, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "skills", Short: "Manage bundled agent skills"}
	cmd.AddCommand(skillsUpdateCmd(g, out))
	return cmd
}

func skillsUpdateCmd(g *globals, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Refresh bundled agent skills from the aidocs server",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client(g)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			paths, err := installBundledSkillFromServer(ctx, c)
			if err != nil {
				return err
			}
			if g.quiet {
				return nil
			}
			if g.json {
				b, err := json.Marshal(map[string]any{"items": []any{map[string]any{"name": "aidocs", "status": "updated", "paths": paths}}})
				if err != nil {
					return err
				}
				return render(out, g, b)
			}
			if err := confirm(out, g, "Updated aidocs skill from "+c.Base); err != nil {
				return err
			}
			for _, p := range paths {
				_, _ = fmt.Fprintln(out, "  "+p)
			}
			return nil
		},
	}
}
