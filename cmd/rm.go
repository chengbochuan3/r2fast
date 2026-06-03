package cmd

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chengbochuan3/r2fast/internal/config"
)

var rmYes bool

var rmCmd = &cobra.Command{
	Use:   "rm <key-or-url> [more...]",
	Short: "Delete uploaded files from R2 now",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runRm,
}

func init() {
	rmCmd.Flags().BoolVarP(&rmYes, "yes", "y", false, "skip confirmation")
}

func runRm(cmd *cobra.Command, args []string) error {
	client, cfg, err := newClient()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(args))
	for _, a := range args {
		keys = append(keys, keyFromArg(cfg, a))
	}

	if !rmYes {
		fmt.Println("About to delete:")
		for _, k := range keys {
			fmt.Println("  ", k)
		}
		fmt.Print("Proceed? [y/N]: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if s := strings.ToLower(strings.TrimSpace(line)); s != "y" && s != "yes" {
			fmt.Println("aborted")
			return nil
		}
	}

	ctx := context.Background()
	for _, k := range keys {
		if err := client.Delete(ctx, k); err != nil {
			fmt.Fprintf(os.Stderr, "failed: %s (%v)\n", k, err)
			continue
		}
		fmt.Println("deleted", k)
	}
	return nil
}

// keyFromArg turns a download URL or a raw key into an object key.
func keyFromArg(cfg *config.Config, arg string) string {
	arg = strings.TrimSpace(arg)
	if u, err := url.Parse(arg); err == nil && u.Scheme != "" && u.Host != "" {
		p := strings.TrimLeft(u.Path, "/")
		if dec, err := url.PathUnescape(p); err == nil {
			return dec
		}
		return p
	}
	return strings.TrimLeft(arg, "/")
}
