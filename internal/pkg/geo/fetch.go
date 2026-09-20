package geo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/oschwald/geoip2-golang/v2"
)

// fetchTimeout bounds the whole download. Nothing reads the database until it
// lands, so this is a boot budget rather than a request budget; GeoLite2-City
// is around 70 MB and a slow link still has to finish.
const fetchTimeout = 3 * time.Minute

// maxDatabaseBytes is what a misconfigured URL is allowed to cost, applied to
// the decoded database rather than the transfer: gzip expands, so a cap on what
// arrives is no cap at all on what lands. GeoLite2-City is the largest edition
// anyone points this at, by a wide margin.
const maxDatabaseBytes = 512 << 20

// tarMagicOffset is where the POSIX ustar magic sits in a 512-byte tar header.
const tarMagicOffset = 257

// maxAge is how long a database we manage is trusted without asking whether a
// newer one exists. MaxMind publishes GeoLite2 on Tuesdays and Fridays, so a
// week is one full cycle of slack.
//
// Only a database this package downloaded ages. With no URL configured nothing
// here runs at all, so an operator's own file is never touched.
const maxAge = 7 * 24 * time.Hour

// Retry budget for the download. A single blip used to cost the container its
// geo data for the whole of its life, because Ensure ran once at boot and
// never again.
const (
	fetchAttempts = 4
	retryBackoff  = 2 * time.Second
	maxRetryWait  = 30 * time.Second
)

// errNotModified is the mirror confirming our copy is still current. MaxMind
// does not count a 304 against the daily download allowance, which is what
// makes checking for staleness cheap enough to do on every boot.
var errNotModified = errors.New("geo: not modified")

// Ensure puts a MaxMind database at path, downloading it from url when nothing
// is there yet and refreshing it once it goes stale. It reports whether it
// wrote a new one.
//
// An empty url does nothing and is not an error, which is how an operator keeps
// a file of their own: nothing here reads or replaces a path this package was
// not asked to manage. The database is optional everywhere it is read, so every
// failure is a warning to the caller and never a reason to refuse to start, and
// a refresh that fails leaves the copy already on disk in place.
//
// A file younger than maxAge is taken as current without a request at all. An
// older one is revalidated with If-Modified-Since, so the usual answer is a 304
// that costs nothing and does not count against MaxMind's daily allowance.
// Without this an existing file won forever, and anything that persisted a copy
// once -- a volume, a mirror, a container that stayed up -- pinned itself to
// that copy for good.
func Ensure(ctx context.Context, path, url string) (bool, error) {
	path, url = strings.TrimSpace(path), strings.TrimSpace(url)
	if path == "" || url == "" {
		return false, nil
	}
	var since time.Time
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		if time.Since(info.ModTime()) < maxAge {
			return false, nil
		}
		since = info.ModTime()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("geo: create %s: %w", dir, err)
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	body, err := get(ctx, url, since)
	if err != nil {
		if errors.Is(err, errNotModified) {
			// Still current. Restamp it so the next boot does not ask again
			// for another maxAge.
			now := time.Now()
			_ = os.Chtimes(path, now, now)
			return false, nil
		}
		return false, err
	}
	defer body.Close()

	// Written beside the destination rather than in a temp dir, so the rename
	// below stays on one filesystem and is atomic. A reader can therefore only
	// ever see a complete database.
	tmp, err := os.CreateTemp(dir, ".geodb-*")
	if err != nil {
		return false, fmt.Errorf("geo: temp file in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name())

	src, err := decode(body)
	if err != nil {
		tmp.Close()
		return false, err
	}
	// One byte past the cap, so an oversized stream is detected rather than
	// silently truncated into a file that then fails to open.
	written, err := io.Copy(tmp, io.LimitReader(src, maxDatabaseBytes+1))
	if err != nil {
		tmp.Close()
		return false, fmt.Errorf("geo: write %s: %w", tmp.Name(), err)
	}
	if written > maxDatabaseBytes {
		tmp.Close()
		return false, fmt.Errorf("geo: %s expands past %d bytes, which no database does", redactURL(url), maxDatabaseBytes)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("geo: write %s: %w", tmp.Name(), err)
	}

	// Opened before it is put in place: a truncated or wrong-format download
	// otherwise becomes the file that "already exists" on every later boot,
	// and the fetch would never be retried.
	db, err := geoip2.Open(tmp.Name())
	if err != nil {
		return false, fmt.Errorf("geo: %s did not serve a MaxMind database: %w", redactURL(url), err)
	}
	_ = db.Close()

	if err := os.Rename(tmp.Name(), path); err != nil {
		return false, fmt.Errorf("geo: install %s: %w", path, err)
	}
	return true, nil
}

// get performs the download, treating any non-2xx as a failure rather than
// writing an error page to disk as if it were a database.
//
// A refused or dropped attempt is retried with a growing wait, because the one
// thing this must not do is give up on the first blip: the caller runs at boot,
// and a container that misses its database here has no geo data until it is
// replaced. A server that says when to come back is obeyed rather than guessed
// at, which is the difference between backing off a 429 and compounding it.
func get(ctx context.Context, raw string, since time.Time) (io.ReadCloser, error) {
	// A URL that cannot carry a credential safely is wrong however many times
	// it is asked, so this is checked once and outside the loop.
	if err := checkURL(raw); err != nil {
		return nil, err
	}

	wait := retryBackoff
	var last error
	for attempt := 1; ; attempt++ {
		body, after, err := fetchOnce(ctx, raw, since)
		if err == nil || errors.Is(err, errNotModified) {
			return body, err
		}
		last = err
		if attempt >= fetchAttempts || !worthRetrying(err) {
			return nil, last
		}
		delay := wait
		if after > 0 {
			delay = after
		}
		if delay > maxRetryWait {
			delay = maxRetryWait
		}
		select {
		case <-ctx.Done():
			return nil, last
		case <-time.After(delay):
		}
		wait *= 2
	}
}

// fetchOnce is one attempt. It returns the server's Retry-After alongside the
// error so the caller can wait exactly as long as it was told to.
func fetchOnce(ctx context.Context, raw string, since time.Time) (io.ReadCloser, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("geo: %s is not a usable URL", redactURL(raw))
	}
	if !since.IsZero() {
		req.Header.Set("If-Modified-Since", since.UTC().Format(http.TimeFormat))
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("geo: %s: %w", redactURL(raw), cause(err))
	}
	if resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return nil, 0, errNotModified
	}
	if resp.StatusCode/100 != 2 {
		after := retryAfter(resp.Header.Get("Retry-After"))
		resp.Body.Close()
		return nil, after, &statusError{url: redactURL(raw), status: resp.Status, code: resp.StatusCode}
	}
	return resp.Body, 0, nil
}

// statusError carries the code so worthRetrying can read it without matching
// on the sentence.
type statusError struct {
	url    string
	status string
	code   int
}

func (e *statusError) Error() string { return fmt.Sprintf("geo: %s returned %s", e.url, e.status) }

// worthRetrying separates "ask again" from "asking again cannot help". A
// refusal the server will repeat -- a bad licence key, a missing edition -- is
// not worth three more of the daily allowance.
func worthRetrying(err error) bool {
	var se *statusError
	if errors.As(err, &se) {
		return se.code == http.StatusTooManyRequests || se.code >= 500
	}
	// A name that does not resolve will not resolve on the third try either,
	// and a mistyped mirror should cost a boot one failed lookup rather than
	// the whole backoff ladder.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false
	}
	// Anything else that is not an answer at all is a transport failure worth
	// another attempt: a reset, a timeout, a mirror still coming up.
	return true
}

// retryAfter reads the header in both the forms RFC 9110 allows. Anything else
// is no guidance, and the caller falls back to its own backoff.
func retryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if when, err := http.ParseTime(v); err == nil {
		if d := time.Until(when); d > 0 {
			return d
		}
	}
	return 0
}

// client refuses to follow a redirect down from https to http, because the
// query string it would carry there is the licence key in cleartext.
var client = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return errors.New("redirected from https to http")
		}
		return nil
	},
}

// checkURL refuses a URL that would put a credential on the wire in the clear.
// http is allowed for a plain mirror, because a self-hosted one on a private
// network is a reasonable thing to have; it is refused the moment the URL
// carries anything secret, which is what a licence key in the query is.
func checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("geo: the configured URL cannot be parsed")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if u.User != nil || u.RawQuery != "" {
			return fmt.Errorf("geo: %s would send a credential in cleartext; use https", redactURL(raw))
		}
		return nil
	default:
		return fmt.Errorf("geo: %s is not an http or https URL", redactURL(raw))
	}
}

// cause strips the URL that net/http puts in its own error text. *url.Error
// prints the address it was given, licence key and all, so wrapping one with
// %w defeats the redaction applied to the URL beside it. Its cause is the part
// worth reading ("connection refused", "no such host") and names nothing.
func cause(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}

// decode unwraps whatever the URL served down to the database bytes. The shape
// is read from the content, not from the URL, because the same file is served
// under every naming convention there is: MaxMind's permalink hands back a
// .tar.gz with the database nested under a dated directory, DB-IP serves a bare
// .mmdb.gz, and a mirror often serves the .mmdb itself.
func decode(r io.Reader) (io.Reader, error) {
	head, rest, err := peek(r, 2)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(head, []byte{0x1f, 0x8b}) {
		return rest, nil
	}
	gz, err := gzip.NewReader(rest)
	if err != nil {
		return nil, fmt.Errorf("geo: gzip: %w", err)
	}
	block, plain, err := peek(gz, tarMagicOffset+5)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(block[tarMagicOffset:], []byte("ustar")) {
		return plain, nil
	}
	return firstDatabase(tar.NewReader(plain))
}

// firstDatabase positions the archive on its first .mmdb member. MaxMind ships
// one per archive alongside a licence and a changelog, so there is nothing to
// choose between.
//
// AppleDouble sidecars are skipped rather than matched. An archive rolled up on
// macOS carries a ._name companion holding each file's extended attributes, it
// is a regular file, it sorts ahead of the file it belongs to, and ._db.mmdb
// ends in .mmdb like any other: taking the first match installs 249 bytes of
// xattrs as the database.
func firstDatabase(tr *tar.Reader) (io.Reader, error) {
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("geo: archive holds no .mmdb file")
		}
		if err != nil {
			return nil, fmt.Errorf("geo: read archive: %w", err)
		}
		name := path.Base(filepath.ToSlash(h.Name))
		if h.Typeflag == tar.TypeReg && strings.HasSuffix(name, ".mmdb") && !strings.HasPrefix(name, "._") {
			return tr, nil
		}
	}
}

// peek reads n bytes and returns them along with a reader that still yields
// them, so a stream can be sniffed without being consumed. A stream shorter
// than n is not an error here; the caller's prefix comparison fails instead.
func peek(r io.Reader, n int) ([]byte, io.Reader, error) {
	buf := make([]byte, n)
	read, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, nil, fmt.Errorf("geo: read: %w", err)
	}
	buf = buf[:read]
	if len(buf) < n {
		buf = append(buf, make([]byte, n-len(buf))...)
	}
	return buf, io.MultiReader(bytes.NewReader(buf[:read]), r), nil
}

// redactURL keeps a download URL out of logs and error strings with its shape
// intact and nothing else. MaxMind's permalink carries the account's licence
// key in the query, a mirror can carry basic-auth credentials in the userinfo,
// and an error message is the one place nobody expects to find either.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable url>"
	}
	u.User = nil
	u.Fragment = ""
	if u.RawQuery != "" {
		u.RawQuery = "<redacted>"
	}
	return u.String()
}
