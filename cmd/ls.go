package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/chengbochuan3/r2fast/internal/util"
)

var lsPrefix string

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List uploaded files and their links",
	RunE:  runLs,
}

func init() {
	lsCmd.Flags().StringVar(&lsPrefix, "prefix", "", "only list keys under this prefix (default: configured prefix)")
}

func runLs(cmd *cobra.Command, args []string) error {
	client, cfg, err := newClient()
	if err != nil {
		return err
	}
	prefix := lsPrefix
	if prefix == "" {
		prefix = cfg.BasePrefix()
	}
	objs, err := client.List(context.Background(), prefix)
	if err != nil {
		return err
	}
	if len(objs) == 0 {
		fmt.Println("no files found")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "SIZE\tAGE\tURL")
	for _, o := range objs {
		fmt.Fprintf(w, "%s\t%s\t%s\n", util.HumanSize(o.Size), util.HumanAge(time.Since(o.LastModified)), o.URL)
	}
	return w.Flush()
}
