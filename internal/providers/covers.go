package providers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	// The three formats we size with image.DecodeConfig. A format we cannot size cannot be bounded, so it is
	// refused rather than stored.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// The cover limits from docs/STANDARDS.md 4a, and the freshness rule from G3.
const (
	// MaxCoverBytes bounds one cover download.
	MaxCoverBytes = 2 << 20
	// MaxCoverSide bounds either dimension; a decompression bomb is refused before it is written.
	MaxCoverSide = 4_000
	// CoverMaxAge is the TMDB terms' six-month caching cap, made a little stricter. A cover older than this is
	// fetched again the next time it is used, so freshness needs no background job (G3, decisions 2).
	CoverMaxAge = 180 * 24 * time.Hour
)

// coverTypes are the content types a cover may have, with the extension each is stored under.
var coverTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/gif":  "gif",
}

// Cover is a stored cover: a path relative to the data directory, and what it is.
type Cover struct {
	Path        string
	ContentType string
	Bytes       int
	FetchedAt   time.Time
}

// Cover keeps a local copy of a cover, fetching it only when there is no fresh one.
//
// Nothing here trusts the URL: its origin must be one the config named, and the bytes must be an image whose dimensions
// can be read. A failed download writes nothing, so the card falls back to its placeholder rather than to a broken file.
//
// The origin rule is scheme, host and port together, not a host alone. The dev and test world names a loopback host on a
// port, and every other service on the board shares that host: a host allow-list would turn a cover URL into a fetch of
// whatever answers locally, and the thumb route feeds this from a page.
func (f *Finder) Cover(ctx context.Context, provider, id, coverURL string) (Cover, error) {
	// The provider name becomes part of a stored file's name, so it has to be one this finder was built with. The
	// thumb route takes it straight from a query, and a name is not a path.
	if _, ok := f.fetchers[provider]; !ok {
		return Cover{}, &Error{
			Provider: provider, Op: "cover", Reason: ReasonUnavailable,
			Err: fmt.Errorf("provider %q is not one this finder knows", provider),
		}
	}
	if cover, ok := f.freshCover(provider, id); ok {
		return cover, nil
	}
	if strings.TrimSpace(coverURL) == "" {
		return Cover{}, &Error{Provider: provider, Op: "cover", Reason: ReasonNotFound, Err: errors.New("no cover")}
	}
	parsed, err := url.Parse(coverURL)
	if err != nil || parsed.Host == "" {
		return Cover{}, &Error{Provider: provider, Op: "cover", Reason: ReasonUnavailable, Err: errors.New("cover URL is not a URL")}
	}
	if !f.coverOriginAllowed(parsed) {
		return Cover{}, &Error{
			Provider: provider, Op: "cover", Reason: ReasonUnauthorized,
			Err: fmt.Errorf("cover origin %q is not one the config names", parsed.Scheme+"://"+parsed.Host),
		}
	}

	if waitErr := f.covers.limiter.wait(ctx); waitErr != nil {
		return Cover{}, f.covers.failure(ctx, "cover", waitErr)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Cover{}, &Error{Provider: provider, Op: "cover", Reason: ReasonUnavailable, Err: err}
	}
	req.Header.Set("User-Agent", f.covers.userAgent)
	req.Header.Set("Accept", "image/*")
	resp, err := f.covers.http.Do(req)
	if err != nil {
		return Cover{}, f.covers.failure(ctx, "cover", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Cover{}, &Error{Provider: provider, Op: "cover", Reason: ReasonNotFound, Status: resp.StatusCode}
	case resp.StatusCode != http.StatusOK:
		return Cover{}, f.covers.failure(ctx, "cover", fmt.Errorf("cover download answered %d", resp.StatusCode))
	}

	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	ext, ok := coverTypes[contentType]
	if !ok {
		return Cover{}, &Error{
			Provider: provider, Op: "cover", Reason: ReasonNotAnImage,
			Err: fmt.Errorf("cover content type %q", contentType),
		}
	}
	if resp.ContentLength > MaxCoverBytes {
		return Cover{}, &Error{
			Provider: provider, Op: "cover", Reason: ReasonCoverTooBig,
			Err: fmt.Errorf("cover is %d bytes", resp.ContentLength),
		}
	}

	body, err := readCoverBody(resp.Body)
	if err != nil {
		if errors.Is(err, errTooLarge) {
			return Cover{}, &Error{Provider: provider, Op: "cover", Reason: ReasonCoverTooBig, Err: err}
		}
		return Cover{}, f.covers.failure(ctx, "cover", err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return Cover{}, &Error{Provider: provider, Op: "cover", Reason: ReasonNotAnImage, Err: err}
	}
	if config.Width > MaxCoverSide || config.Height > MaxCoverSide {
		return Cover{}, &Error{
			Provider: provider, Op: "cover", Reason: ReasonCoverTooBig,
			Err: fmt.Errorf("cover is %dx%d", config.Width, config.Height),
		}
	}

	stored, err := f.writeCover(provider, id, ext, body)
	if err != nil {
		return Cover{}, &Error{Provider: provider, Op: "cover", Reason: ReasonUnavailable, Err: err}
	}
	return Cover{
		Path:        stored,
		ContentType: contentType,
		Bytes:       len(body),
		FetchedAt:   f.clock.Now(),
	}, nil
}

// CoverDir is where covers live inside the data directory. It is the one place that name appears.
const CoverDir = "covers"

// ThumbKey names a candidate's thumbnail in the cover cache. It is a hash of the source URL and never the id a request
// names: a page that could choose the stored name could write a new file for every id it invented, and nothing in the
// tree evicts those. One source is one file, however many times it is drawn.
func thumbKey(coverURL string) string {
	sum := sha256.Sum256([]byte(coverURL))
	return "thumb-" + hex.EncodeToString(sum[:8])
}

// CandidateCover stores a candidate's thumbnail. It is Cover with a key derived from the source, because the URL comes
// from a page rather than from a provider response she is logging.
func (f *Finder) CandidateCover(ctx context.Context, provider, coverURL string) (Cover, error) {
	return f.Cover(ctx, provider, thumbKey(coverURL), coverURL)
}

// ThumbPath is the app-served URL a candidate's cover is drawn from. The page's CSP allows images from this origin
// only, so a payload that carried the provider's own URL drew nothing at all; the route fetches the provider's copy
// through Cover and serves it from here. The source travels with the path because only the lookup that found the
// candidate knows it, and the route validates it again before anything is downloaded.
func ThumbPath(provider, coverURL string) string {
	if strings.TrimSpace(coverURL) == "" {
		return ""
	}
	query := url.Values{}
	query.Set("provider", provider)
	query.Set("src", coverURL)
	return "/api/thumb?" + query.Encode()
}

// coverName builds the stored file name: provider, a sanitised id and the extension. Both are reduced to letters,
// digits, dash and underscore, with every other run collapsed to one dash, so a name from a provider can never name a
// path outside the cover directory and never produces a name with a dot in it. Cover only calls this with a provider
// it was built with; the sanitising is what makes that belt hold if a later caller forgets.
func coverName(provider, id, ext string) string {
	part := fileNamePart(provider)
	key := fileNamePart(id)
	if key == "" {
		key = "unnamed"
	}
	name := part + "-" + key
	if ext != "" {
		name += "." + ext
	}
	return name
}

// fileNamePart reduces one part of a stored file's name to letters, digits, dash and underscore.
func fileNamePart(part string) string {
	var b strings.Builder
	b.Grow(len(part))
	lastDash := false
	for _, r := range part {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-_")
}

// freshCover finds an existing cover that is still inside the freshness window.
func (f *Finder) freshCover(provider, id string) (Cover, bool) {
	dir := filepath.Join(f.dataDir, CoverDir)
	for contentType, ext := range coverTypes {
		full := filepath.Join(dir, coverName(provider, id, ext))
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}
		if f.clock.Now().Sub(info.ModTime()) >= CoverMaxAge {
			continue
		}
		return Cover{
			Path:        path.Join(CoverDir, coverName(provider, id, ext)),
			ContentType: contentType,
			Bytes:       int(info.Size()),
			FetchedAt:   info.ModTime(),
		}, true
	}
	return Cover{}, false
}

// writeCover stores the bytes atomically, so a crash leaves either the old cover or the new one, never half of a
// file. The modification time is the fetch time, which is what the freshness rule reads.
func (f *Finder) writeCover(provider, id, ext string, body []byte) (string, error) {
	dir := filepath.Join(f.dataDir, CoverDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	name := coverName(provider, id, ext)
	temp, err := os.CreateTemp(dir, ".cover-*")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
	}()
	if _, err := temp.Write(body); err != nil {
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	full := filepath.Join(dir, name)
	if err := os.Rename(temp.Name(), full); err != nil {
		return "", err
	}
	// The freshness rule reads the file's own modification time, so set it from the injected clock instead of
	// leaving it to the machine's: a test can then move time, and a clock skew cannot quietly extend the cache.
	now := f.clock.Now()
	if err := os.Chtimes(full, now, now); err != nil {
		return "", err
	}
	return path.Join(CoverDir, name), nil
}

// readCoverBody reads at most the cover limit and refuses one byte more.
func readCoverBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, MaxCoverBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxCoverBytes {
		return nil, errTooLarge
	}
	return body, nil
}

// coverOriginAllowed reports whether a cover URL is one the config named: same scheme, host and port as one of the
// configured cover bases. A userinfo part is refused too, so a URL cannot smuggle a credential into a fetch or into the
// name it is stored under.
func (f *Finder) coverOriginAllowed(parsed *url.URL) bool {
	if parsed.User != nil {
		return false
	}
	origin := originOf(parsed)
	for _, allowed := range f.coverOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

// originOf is the scheme, host and port of a URL, lower-cased so a host's case cannot matter.
func originOf(parsed *url.URL) string {
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

// coverOriginsFrom reads the origins of the configured cover bases. A base's path is not part of it: the size prefix
// TMDB appends is not a security boundary, and a provider may change it without changing what it is.
func coverOriginsFrom(bases ...string) []string {
	out := make([]string, 0, len(bases))
	for _, base := range bases {
		parsed, err := url.Parse(base)
		if err != nil || parsed.Host == "" {
			continue
		}
		out = append(out, originOf(parsed))
	}
	return out
}

// coverHostsFrom reads the hosts of the configured cover bases.
func coverHostsFrom(bases ...string) map[string]bool {
	out := make(map[string]bool, len(bases))
	for _, base := range bases {
		parsed, err := url.Parse(base)
		if err != nil || parsed.Hostname() == "" {
			continue
		}
		out[parsed.Hostname()] = true
	}
	return out
}
