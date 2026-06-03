package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/chengbochuan3/r2fast/internal/config"
	"github.com/chengbochuan3/r2fast/internal/r2"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage r2fast configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive setup wizard (credentials, bucket, domain)",
	RunE:  runConfigInit,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration (secret masked)",
	RunE:  runConfigShow,
}

func init() {
	configCmd.AddCommand(configInitCmd, configShowCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	existing, _ := config.Load()
	r := bufio.NewReader(os.Stdin)

	fmt.Println("r2fast setup — stored in", config.Path())
	fmt.Println("(Cloudflare dashboard -> R2 -> Manage R2 API Tokens for the keys)")
	fmt.Println()

	c := &config.Config{}
	c.AccountID = prompt(r, "R2 Account ID (hex in your endpoint URL)", existing.AccountID)
	c.AccessKeyID = prompt(r, "Access Key ID", existing.AccessKeyID)

	defSecret := ""
	if existing.SecretAccessKey != "" {
		defSecret = "keep existing"
	}
	c.SecretAccessKey = readSecret("Secret Access Key", defSecret)
	if c.SecretAccessKey == "" {
		c.SecretAccessKey = existing.SecretAccessKey
	}

	c.Bucket = prompt(r, "Bucket name", existing.Bucket)
	c.PublicBaseURL = strings.TrimRight(prompt(r, "Download domain (e.g. https://files.example.com)", existing.PublicBaseURL), "/")
	c.Prefix = prompt(r, "Key prefix (optional, blank = bucket root)", existing.Prefix)
	c.DefaultTTL = prompt(r, "Default auto-delete (e.g. 7d, 30d, none)", firstNonEmpty(existing.DefaultTTL, "7d"))
	c.RandomSuffix = yesNo(r, "Add a random token to links by default (harder to guess)?", existing.RandomSuffix)
	c.Expiry = chooseExpiry(r, existing.Expiry)
	c.ExpirePrefix = existing.ExpirePrefix
	c.PartSizeMB = existing.PartSizeMB
	c.Concurrency = existing.Concurrency

	if err := c.Validate(); err != nil {
		return err
	}
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Println("\nSaved", config.Path())

	ctx := context.Background()
	client, err := r2.New(ctx, c)
	if err != nil {
		return err
	}
	fmt.Print("Testing access... ")
	if err := client.VerifyAccess(ctx); err != nil {
		fmt.Println("FAILED")
		fmt.Fprintf(os.Stderr, "  %v\n  Double-check the keys, bucket name and account ID.\n", err)
		return nil
	}
	fmt.Println("ok")

	if c.ExpiryMode() != "worker" { // lifecycle or auto: set up the day-based rules
		fmt.Print("Setting up auto-expiry rules (1/3/7/14/30 days)... ")
		if _, err := client.EnsureLifecycleFor(ctx, 1, 3, 7, 14, 30); err != nil {
			fmt.Println("skipped")
			if r2.IsAccessDenied(err) {
				fmt.Fprintln(os.Stderr, "  This API token can't manage lifecycle rules (needs R2 'Admin' permission).")
				fmt.Fprintln(os.Stderr, "  Uploads, links and deletes still work — files just won't auto-expire.")
				fmt.Fprintln(os.Stderr, "  For auto-delete: use an Admin token + `r2fast lifecycle ensure`, or add the rule in the R2 dashboard.")
			} else {
				fmt.Fprintf(os.Stderr, "  %v\n", err)
			}
		} else {
			fmt.Println("ok")
		}
	}
	if c.ExpiryMode() != "lifecycle" { // worker or auto: needs the Worker for sub-day TTLs
		fmt.Println("\nFor sub-day TTLs (2h, 30m, ...), deploy the expiry Worker once (no API token):")
		fmt.Println("  cd worker && npm install && npx wrangler deploy   (see worker/README.md)")
	}
	fmt.Println("\nReady. Try:  r2fast upload <file> --ttl 7d")
	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Println("config file:    ", config.Path())
	fmt.Println("account_id:     ", c.AccountID)
	fmt.Println("access_key_id:  ", c.AccessKeyID)
	fmt.Println("secret:         ", maskSecret(c.SecretAccessKey))
	fmt.Println("bucket:         ", c.Bucket)
	fmt.Println("endpoint:       ", c.ResolvedEndpoint())
	fmt.Println("public_base_url:", c.PublicBaseURL)
	fmt.Println("prefix:         ", c.Prefix)
	fmt.Println("default_ttl:    ", c.DefaultTTL)
	fmt.Printf("random_suffix:   %v\n", c.RandomSuffix)
	fmt.Println("expiry:         ", c.ExpiryMode())
	fmt.Println("expire_prefix:  ", c.ExpiringPrefix())
	return nil
}

// ---- prompt helpers ----

func prompt(r *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := r.ReadString('\n')
	if line = strings.TrimSpace(line); line != "" {
		return line
	}
	return def
}

func readSecret(label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Println()
		if err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}

func yesNo(r *bufio.Reader, label string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	fmt.Printf("%s [%s]: ", label, hint)
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return def
	case "y", "yes":
		return true
	default:
		return false
	}
}

func chooseExpiry(r *bufio.Reader, def string) string {
	if def == "" {
		def = "auto"
	}
	fmt.Printf("Auto-delete mode — 'auto' (days->lifecycle, sub-day->worker), 'lifecycle' (days only, no server), or 'worker' (always precise) [%s]: ", def)
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return def
	case "worker", "w":
		return "worker"
	case "lifecycle", "l":
		return "lifecycle"
	case "auto", "a":
		return "auto"
	default:
		return def
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func maskSecret(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}
