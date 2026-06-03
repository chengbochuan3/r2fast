package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/chengbochuan3/r2fast/internal/config"
	"github.com/chengbochuan3/r2fast/internal/r2"
)

var lifecycleDays []int

var lifecycleCmd = &cobra.Command{
	Use:   "lifecycle",
	Short: "Inspect or create R2 auto-expiry rules",
}

var lifecycleShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current lifecycle (auto-delete) rules on the bucket",
	RunE:  runLifecycleShow,
}

var lifecycleEnsureCmd = &cobra.Command{
	Use:   "ensure",
	Short: "Create/verify auto-expiry rules for the given day tiers",
	RunE:  runLifecycleEnsure,
}

func init() {
	lifecycleEnsureCmd.Flags().IntSliceVar(&lifecycleDays, "days", []int{1, 3, 7, 14, 30}, "day tiers to ensure rules for")
	lifecycleCmd.AddCommand(lifecycleShowCmd, lifecycleEnsureCmd)
}

// newClient is shared by the read/write subcommands.
func newClient() (*r2.Client, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	client, err := r2.New(context.Background(), cfg)
	return client, cfg, err
}

func runLifecycleShow(cmd *cobra.Command, args []string) error {
	client, _, err := newClient()
	if err != nil {
		return err
	}
	infos, err := client.ListLifecycle(context.Background())
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Println("no lifecycle rules on this bucket")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPREFIX\tEXPIRE(days)\tSTATUS")
	for _, i := range infos {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", i.ID, i.Prefix, i.Days, i.Status)
	}
	return w.Flush()
}

func runLifecycleEnsure(cmd *cobra.Command, args []string) error {
	client, _, err := newClient()
	if err != nil {
		return err
	}
	changed, err := client.EnsureLifecycleFor(context.Background(), lifecycleDays...)
	if err != nil {
		return err
	}
	if changed {
		fmt.Println("lifecycle rules updated")
	} else {
		fmt.Println("lifecycle rules already in place")
	}
	return nil
}
