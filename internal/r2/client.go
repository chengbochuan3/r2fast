// Package r2 wraps the S3-compatible Cloudflare R2 API: multipart uploads,
// lifecycle (auto-expiry) rules, listing and deletion.
package r2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"

	"github.com/chengbochuan3/r2fast/internal/config"
)

type Client struct {
	s3  *s3.Client
	cfg *config.Config
}

// New builds an R2 client from config. The aws.Config is constructed directly
// (no LoadDefaultConfig) so the tool never picks up ambient AWS profiles/env
// credentials or probes EC2 IMDS.
func New(ctx context.Context, cfg *config.Config) (*Client, error) {
	_ = ctx
	tr := http.DefaultTransport.(*http.Transport).Clone()
	awsCfg := aws.Config{
		Region:      "auto",
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		HTTPClient:  &http.Client{Transport: &countingTransport{base: tr}},
	}
	cl := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.ResolvedEndpoint())
		o.UsePathStyle = true
		// R2 rejects the SDK's default trailing CRC checksums on some paths;
		// only send checksums when the operation strictly requires them.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	return &Client{s3: cl, cfg: cfg}, nil
}

type UploadResult struct {
	Key  string
	URL  string
	Size int64
}

// Upload streams a local file to R2 using multipart, reporting bytes read via
// onProgress (may be nil).
func (c *Client) Upload(ctx context.Context, localPath, key string, meta map[string]string, onProgress func(n int)) (*UploadResult, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}

	uploader := manager.NewUploader(c.s3, func(u *manager.Uploader) {
		if c.cfg.PartSizeMB > 0 {
			u.PartSize = c.cfg.PartSizeMB * 1024 * 1024
		}
		if c.cfg.Concurrency > 0 {
			u.Concurrency = c.cfg.Concurrency
		}
	})
	in := &s3.PutObjectInput{
		Bucket:      aws.String(c.cfg.Bucket),
		Key:         aws.String(key),
		Body:        f, // seekable: the manager streams parts from the file, no full-buffering
		ContentType: aws.String(guessContentType(localPath)),
	}
	if len(meta) > 0 {
		in.Metadata = meta
	}
	// Progress is measured at the HTTP transport (bytes actually written to the
	// socket), not at disk-read time — so the bar tracks the real upload.
	if _, err := uploader.Upload(withProgress(ctx, onProgress), in); err != nil {
		return nil, err
	}
	return &UploadResult{Key: key, URL: c.PublicURL(key), Size: st.Size()}, nil
}

// PublicURL builds the download link for a key.
func (c *Client) PublicURL(key string) string {
	enc := encodeKey(key)
	if base := strings.TrimRight(c.cfg.PublicBaseURL, "/"); base != "" {
		return base + "/" + enc
	}
	return fmt.Sprintf("%s/%s/%s", strings.TrimRight(c.cfg.ResolvedEndpoint(), "/"), c.cfg.Bucket, enc)
}

// ---- lifecycle (auto-expiry) ----

func ruleID(days int) string { return fmt.Sprintf("r2fast-%dd", days) }

func (c *Client) lifecycleRule(days int) types.LifecycleRule {
	return types.LifecycleRule{
		ID:         aws.String(ruleID(days)),
		Status:     types.ExpirationStatusEnabled,
		Filter:     &types.LifecycleRuleFilter{Prefix: aws.String(TierPrefix(c.cfg.BasePrefix(), days))},
		Expiration: &types.LifecycleExpiration{Days: aws.Int32(int32(days))},
	}
}

func (c *Client) getLifecycleRules(ctx context.Context) ([]types.LifecycleRule, error) {
	out, err := c.s3.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{
		Bucket: aws.String(c.cfg.Bucket),
	})
	if err != nil {
		if isNoSuchLifecycle(err) {
			return nil, nil
		}
		return nil, err
	}
	return out.Rules, nil
}

func (c *Client) putLifecycleRules(ctx context.Context, rules []types.LifecycleRule) error {
	_, err := c.s3.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket:                 aws.String(c.cfg.Bucket),
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: rules},
	})
	return err
}

// EnsureLifecycleFor makes sure an expiry rule exists for each given day tier,
// preserving any unrelated rules already on the bucket. Returns whether a
// change was written. When all requested rules already exist it makes no
// mutating call (so an object-only token still works).
func (c *Client) EnsureLifecycleFor(ctx context.Context, days ...int) (bool, error) {
	rules, err := c.getLifecycleRules(ctx)
	if err != nil {
		return false, err
	}
	have := map[string]bool{}
	for _, r := range rules {
		have[aws.ToString(r.ID)] = true
	}
	changed := false
	for _, d := range days {
		if d < 1 || have[ruleID(d)] {
			continue
		}
		rules = append(rules, c.lifecycleRule(d))
		have[ruleID(d)] = true
		changed = true
	}
	if !changed {
		return false, nil
	}
	if err := c.putLifecycleRules(ctx, rules); err != nil {
		return false, err
	}
	return true, nil
}

type LifecycleInfo struct {
	ID     string
	Prefix string
	Days   int32
	Status string
}

func (c *Client) ListLifecycle(ctx context.Context) ([]LifecycleInfo, error) {
	rules, err := c.getLifecycleRules(ctx)
	if err != nil {
		return nil, err
	}
	var out []LifecycleInfo
	for _, r := range rules {
		info := LifecycleInfo{ID: aws.ToString(r.ID), Status: string(r.Status)}
		if r.Filter != nil && r.Filter.Prefix != nil {
			info.Prefix = aws.ToString(r.Filter.Prefix)
		}
		if r.Expiration != nil && r.Expiration.Days != nil {
			info.Days = aws.ToInt32(r.Expiration.Days)
		}
		out = append(out, info)
	}
	return out, nil
}

func isNoSuchLifecycle(err error) bool {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return strings.Contains(ae.ErrorCode(), "NoSuchLifecycle")
	}
	return false
}

// IsAccessDenied reports whether err is an S3 AccessDenied error — e.g. an
// object-scoped token without bucket-admin permission for lifecycle config.
func IsAccessDenied(err error) bool {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode() == "AccessDenied"
	}
	return false
}

// ---- listing / deletion ----

type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
	URL          string
}

func (c *Client) List(ctx context.Context, prefix string) ([]Object, error) {
	in := &s3.ListObjectsV2Input{Bucket: aws.String(c.cfg.Bucket)}
	if prefix != "" {
		in.Prefix = aws.String(prefix)
	}
	var objs []Object
	p := s3.NewListObjectsV2Paginator(c.s3, in)
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			key := aws.ToString(o.Key)
			objs = append(objs, Object{
				Key:          key,
				Size:         aws.ToInt64(o.Size),
				LastModified: aws.ToTime(o.LastModified),
				URL:          c.PublicURL(key),
			})
		}
	}
	sort.Slice(objs, func(i, j int) bool { return objs[i].LastModified.After(objs[j].LastModified) })
	return objs, nil
}

func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.cfg.Bucket),
		Key:    aws.String(key),
	})
	return err
}

// VerifyAccess checks credentials + bucket reachability.
func (c *Client) VerifyAccess(ctx context.Context) error {
	_, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(c.cfg.Bucket),
		MaxKeys: aws.Int32(1),
	})
	return err
}

// ---- helpers ----

// --- upload progress measured at the HTTP transport layer ---

type progressKeyT struct{}

var progressKey progressKeyT

func withProgress(ctx context.Context, cb func(n int)) context.Context {
	if cb == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey, cb)
}

// countingTransport wraps each request body so bytes are counted as they're
// written to the socket — i.e. true upload progress, not disk-read progress.
type countingTransport struct{ base http.RoundTripper }

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.Body != http.NoBody {
		if cb, ok := req.Context().Value(progressKey).(func(n int)); ok {
			req.Body = &countingBody{ReadCloser: req.Body, cb: cb}
		}
	}
	return t.base.RoundTrip(req)
}

type countingBody struct {
	io.ReadCloser
	cb func(n int)
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.cb(n)
	}
	return n, err
}

func guessContentType(p string) string {
	if ct := mime.TypeByExtension(filepath.Ext(p)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
