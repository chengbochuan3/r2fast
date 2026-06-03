package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/chengbochuan3/r2fast/internal/config"
	"github.com/chengbochuan3/r2fast/internal/r2"
	"github.com/chengbochuan3/r2fast/internal/util"
)

var (
	upTTL     string
	upName    string
	upPrivate bool
	upNoCopy  bool
)

func addUploadFlags(c *cobra.Command) {
	c.Flags().StringVar(&upTTL, "ttl", "", `auto-delete after N days (e.g. "7d", "30d") or "none"; default from config`)
	c.Flags().StringVar(&upName, "name", "", "override the remote file name (single file only)")
	c.Flags().BoolVar(&upPrivate, "private", false, "insert a random token in the link so it can't be guessed")
	c.Flags().BoolVar(&upNoCopy, "no-copy", false, "do not copy the link to the clipboard")
}

var uploadCmd = &cobra.Command{
	Use:     "upload [file...]",
	Aliases: []string{"up"},
	Short:   "Upload one or more files to R2 and print share links",
	Args:    cobra.MinimumNArgs(1),
	RunE:    runUpload,
}

func init() { addUploadFlags(uploadCmd) }

func runUpload(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	ttlStr := upTTL
	if ttlStr == "" {
		ttlStr = cfg.DefaultTTL
	}
	ttl, err := r2.ParseTTL(ttlStr)
	if err != nil {
		return err
	}
	if upName != "" && len(args) > 1 {
		return fmt.Errorf("--name can only be used with a single file")
	}

	ctx := context.Background()
	client, err := r2.New(ctx, cfg)
	if err != nil {
		return err
	}

	// Two expiry mechanisms (see internal/config + worker/):
	//   lifecycle: whole-day TTLs, object under Nd/, deleted by an R2 rule.
	//   worker:    any TTL, object under e/<token>/ + `expire-at` metadata,
	//              deleted by the Cloudflare Worker every minute.
	// Neither is touched per upload here — rules/Worker are set up once.
	mode := cfg.ExpiryMode()
	useWorker := false
	if !ttl.IsKeep() {
		switch mode {
		case "worker":
			useWorker = true
		case "auto":
			_, whole := ttl.WholeDays()
			useWorker = !whole // whole days -> lifecycle (clean Nd/ link); sub-day -> worker
		default: // lifecycle only
			if _, whole := ttl.WholeDays(); !whole {
				return fmt.Errorf("ttl %s needs sub-day precision; set expiry to \"auto\" or \"worker\" and deploy the Worker (worker/README.md), or use whole days like 7d/30d", ttl.Human())
			}
		}
	}

	// Only draw a live progress bar on a real terminal; when piped/redirected
	// the ANSI control codes would just be noise.
	showBar := term.IsTerminal(int(os.Stderr.Fd()))

	var urls []string
	for _, path := range args {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory (folder upload not supported yet)", path)
		}
		name := upName
		if name == "" {
			name = info.Name()
		}

		tier := ""
		token := upPrivate || cfg.RandomSuffix
		var meta map[string]string
		if !ttl.IsKeep() {
			if useWorker {
				tier = cfg.ExpiringPrefix()
				token = true // ephemeral: always token so repeat names don't clash
				meta = map[string]string{"expire-at": strconv.FormatInt(time.Now().Add(ttl.Dur).Unix(), 10)}
			} else {
				days, _ := ttl.WholeDays()
				tier = fmt.Sprintf("%dd", days)
			}
		}
		key := r2.BuildKey(cfg.BasePrefix(), tier, name, token)

		var bar *progressbar.ProgressBar
		var onProgress func(int)
		if showBar {
			bar = progressbar.NewOptions64(info.Size(),
				progressbar.OptionSetDescription(truncate(info.Name(), 24)),
				progressbar.OptionShowBytes(true),
				progressbar.OptionShowCount(),
				progressbar.OptionSetWidth(18),
				progressbar.OptionUseANSICodes(true),  // clear with \033[2K, never space-fill (no line-wrap cascade)
				progressbar.OptionSetPredictTime(false), // drop the variable-length [elapsed:remaining] so the line stays short
				progressbar.OptionThrottle(90*time.Millisecond),
				progressbar.OptionSetWriter(os.Stderr),
				progressbar.OptionOnCompletion(func() { fmt.Fprintln(os.Stderr) }),
			)
			onProgress = func(n int) { _ = bar.Add(n) }
		}
		res, err := client.Upload(ctx, path, key, meta, onProgress)
		if bar != nil {
			_ = bar.Finish()
		}
		if err != nil {
			return fmt.Errorf("upload %s: %w", path, err)
		}
		urls = append(urls, res.URL)
		fmt.Println(res.URL)
	}

	if !upNoCopy && len(urls) > 0 {
		if err := util.CopyClipboard(strings.Join(urls, "\n")); err == nil {
			fmt.Fprintln(os.Stderr, "(link copied to clipboard)")
		}
	}
	if !ttl.IsKeep() {
		if useWorker {
			at := time.Now().Add(ttl.Dur).Format("2006-01-02 15:04")
			fmt.Fprintf(os.Stderr, "auto-deletes ~%s (in %s) — needs the expiry Worker deployed (worker/README.md)\n", at, ttl.Human())
		} else {
			days, _ := ttl.WholeDays()
			fmt.Fprintf(os.Stderr, "auto-deletes in ~%d day(s) — needs the %dd lifecycle rule (verify: r2fast lifecycle show)\n", days, days)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
