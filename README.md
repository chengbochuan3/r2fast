# r2fast

Drag a file in, get a fast download link back. `r2fast` uploads local files
straight to your **Cloudflare R2** bucket and prints a download URL served from
**your own domain** (no `r2.dev` rate limits). Files can **auto-delete after N
days**. Built for the "my dataset is 40 GB and `scp` to the GPU box is painful"
problem — push once to R2, pull fast from anywhere.

```
$ r2fast upload dataset.tar --ttl 7d
dataset.tar  1.2 GB ████████████████ 100%
https://files.example.com/7d/dataset.tar
(link copied to clipboard)
auto-deletes in ~7 day(s)
```

---

## ⚠️ Security first

- **Your credentials never live in this repo.** They go in a per-user file
  (`~/.config/r2fast/config.toml`, mode `600`) or in `R2FAST_*` environment
  variables. `config.toml` is git-ignored.
- **If a Secret Access Key ever leaks, rotate it** in the Cloudflare dashboard
  (R2 → Manage R2 API Tokens). A leaked secret can read/write your whole bucket.
- **Links are public.** Anyone with the URL can download (that's the point), and
  plain filenames are guessable. Use `--private` (or `random_suffix = true`) to
  add a random token to the path so links can't be guessed.

## What it does

- **Fast upload** — multipart, concurrent, with a progress bar. Same speed as
  `aws s3 cp`, no AWS CLI required.
- **Fast download** — links use your custom R2 domain, which is CDN-backed and
  not rate-limited like the managed `*.r2.dev` URL.
- **Auto-expiry** — set `--ttl 7d` and the object is deleted ~7 days later via an
  R2 lifecycle rule. No server, no cron.
- **Manage** — `ls`, `rm`, and `lifecycle` subcommands.

## Install

**Prebuilt binary** — download for your platform from the
[Releases](https://github.com/chengbochuan3/r2fast/releases) page, unpack, and
put `r2fast` on your `PATH`.

**With Go** (1.22+):

```bash
go install github.com/chengbochuan3/r2fast@latest
```

**From source:**

```bash
git clone https://github.com/chengbochuan3/r2fast.git && cd r2fast
go build -o r2fast . && sudo mv r2fast /usr/local/bin/
```

## Quick start

```bash
r2fast config init      # one-time wizard: account id, keys, bucket, domain
r2fast upload big.tar --ttl 7d
```

`config init` asks for:

| Field | Example | Where to find it |
|---|---|---|
| Account ID | `4dcdbb5f…` | the hex in your R2 endpoint URL |
| Access Key ID / Secret | … | R2 → *Manage R2 API Tokens* |
| Bucket | `my-bucket` | your R2 bucket name |
| Download domain | `https://files.example.com` | the custom domain connected to the bucket |

It then tests access and creates the standard expiry rules (1/3/7/14/30 days).

## Commands

```
r2fast upload <file...> [--ttl 7d] [--name x.tar] [--private] [--no-copy]
r2fast <file...>                 # shorthand for upload (handy for drag-into-terminal)
r2fast ls [--prefix p]           # list uploaded files + links
r2fast rm <key-or-url>... [-y]   # delete now
r2fast lifecycle show            # view auto-delete rules
r2fast lifecycle ensure --days 1,3,7,14,30
r2fast config init | show
```

`--ttl` accepts `7d`, `30d`, a bare number of days, or `none` to keep forever.
In **auto**/**worker** expiry mode it also takes sub-day values: `2h`, `30m`, `1h30m`.

## How it works

An uploaded file's object key encodes its tier:

```
prefix / <N>d / [random-token /] filename
e.g.   7d/dataset.tar        ->  https://<domain>/7d/dataset.tar
       30d/ab12cd34/data.bin  ->  https://<domain>/30d/ab12cd34/data.bin
       model.bin  (ttl none)  ->  https://<domain>/model.bin
```

For each `Nd/` prefix there's an R2 **lifecycle rule** that deletes objects N
days after upload. Create these rules once with `r2fast config init` or
`r2fast lifecycle ensure` (they merge with any rules you already have). Uploads
themselves never touch lifecycle config.

> **Permissions:** creating lifecycle rules needs an R2 API token with **Admin**
> permission. Day-to-day upload/download/delete only needs an **Object Read &
> Write** token. If your token is object-only and no rule exists yet, files
> still upload but **won't auto-expire** — add an Admin token (or create the
> rule in the R2 dashboard), then `r2fast lifecycle show` to confirm.

### Expiry modes

Set `expiry` in your config:

- **`auto`** (default) — whole-day TTLs use lifecycle (clean `7d/…` links),
  sub-day TTLs use the worker. Best of both, no thinking required.
- **`lifecycle`** (zero infra) — whole-day TTLs only (`7d`, `30d`). Files go
  under an `Nd/` prefix and an R2 lifecycle rule deletes them N days later. The
  sweep runs roughly daily, so deletion lands within ~24h of expiry. Setup needs
  an **Admin** token once (`r2fast lifecycle ensure`).
- **`worker`** (always precise) — any TTL down to the minute (`2h`, `30m`,
  `1h30m`). Files go under an `e/` prefix stamped with an `expire-at` timestamp,
  and a small **Cloudflare Worker** ([`worker/`](worker/)) deletes them at expiry
  every minute. No API token involved. Deploy it once — see
  [worker/README.md](worker/README.md).

> After deletion the R2 origin is empty immediately, but Cloudflare's edge cache
> may serve a stale copy of the file for a short time.

## Configuration

`~/.config/r2fast/config.toml` (override the dir with `R2FAST_CONFIG_DIR`):

```toml
account_id       = "..."
access_key_id    = "..."
secret_access_key= "..."
bucket           = "my-bucket"
public_base_url  = "https://files.example.com"
prefix           = ""        # blank = bucket root
default_ttl      = "7d"
random_suffix    = false
expiry           = "auto"       # auto | lifecycle | worker
expire_prefix    = "e"          # worker-mode key prefix
part_size_mb     = 16
concurrency      = 8
```

Every field can also be supplied via env vars (these win over the file), which
is the no-secrets-on-disk way to run in CI or on a shared box:

```
R2FAST_ACCOUNT_ID  R2FAST_ACCESS_KEY_ID  R2FAST_SECRET_ACCESS_KEY
R2FAST_BUCKET  R2FAST_ENDPOINT  R2FAST_PUBLIC_BASE_URL  R2FAST_PREFIX
R2FAST_EXPIRY  R2FAST_EXPIRE_PREFIX
```

## Setting up R2 + a custom domain (once)

1. Create an R2 bucket in the Cloudflare dashboard.
2. **R2 → your bucket → Settings → Custom Domains** → add a domain you control
   (e.g. `files.example.com`). This is what makes downloads fast and unmetered
   vs. the managed `r2.dev` URL.
3. **Manage R2 API Tokens** → create a token with Object Read & Write (add Admin
   if you want `r2fast` to manage lifecycle rules for you).

## Roadmap

- Drag-and-drop TTL picker and a signed/notarized macOS app.
- Second-precision expiry (Durable Object alarms) for worker mode.
- Resumable uploads and Windows/Linux clipboard polish.

## License

MIT — see [LICENSE](LICENSE).
