# r2fast expiry Worker

Deletes precisely-expiring uploads (`--ttl 2h`, `30m`, `1h30m`, …) the minute
they expire. Used only when your r2fast config has `expiry = "worker"`.

**How it works:** the CLI uploads these files under a prefix (default `e/`) and
stamps each object with an `expire-at` (unix seconds) custom-metadata value.
This Worker runs every minute, lists that prefix with metadata included, and
deletes whatever is past due — via the R2 bucket binding, so **no API token is
involved**.

## Deploy (once)

Prereqs: a Cloudflare account and Node. Wrangler runs via `npx`.

1. Edit `wrangler.toml`:
   - `bucket_name` → your R2 bucket (e.g. `my-bucket`)
   - keep `EXPIRE_PREFIX` in sync with `expire_prefix` in your r2fast config
     (default `e/`)
2. From this folder:
   ```bash
   npm install             # installs wrangler (pinned in package.json)
   npx wrangler login      # first time only — opens your browser to authorize
   npx wrangler deploy
   ```

Verify it runs: `npx wrangler tail` (live logs) or Cloudflare dashboard →
Workers → r2fast-expiry → Logs. You can also trigger a run on demand with
`npx wrangler dev --test-scheduled` then hitting `/__scheduled`.

## Notes

- Granularity is ~1 minute (the Cron minimum) — plenty for "share then
  auto-clean" use.
- After deletion the R2 origin is empty immediately, but Cloudflare's edge cache
  can serve a stale copy of the file for a short time.
- Cost: one cron invocation per minute plus one `list` per run — negligible on
  the Workers free plan for small numbers of files.
- Scope: it only ever touches keys under `EXPIRE_PREFIX`. Permanent files and
  day-tier (`7d/`, `30d/`, …) files are never scanned or deleted by this Worker.
