package telegram

import (
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsnbyte/telesend/internal/apperr"
)

const (
	// DefaultMaxPhotoSizeBytes is the Telegram Bot API photo upload limit (10 MB).
	DefaultMaxPhotoSizeBytes = 10 * 1024 * 1024
	// DefaultMaxMediaSizeBytes is the Telegram Bot API general media upload limit (50 MB).
	DefaultMaxMediaSizeBytes = 50 * 1024 * 1024
)

// MediaFetcher securely downloads media files from URLs with SSRF protection and size limits.
type MediaFetcher struct {
	HTTPClient        *http.Client
	AllowLocalhost    bool
	MaxPhotoSizeBytes int64
	MaxMediaSizeBytes int64
}

// NewMediaFetcher creates a MediaFetcher with SSRF protection enabled.
func NewMediaFetcher(allowLocalhost bool) *MediaFetcher {
	mf := &MediaFetcher{
		AllowLocalhost:    allowLocalhost,
		MaxPhotoSizeBytes: DefaultMaxPhotoSizeBytes,
		MaxMediaSizeBytes: DefaultMaxMediaSizeBytes,
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			var ips []net.IP
			if ip := net.ParseIP(host); ip != nil {
				ips = []net.IP{ip}
			} else {
				resolved, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
				if err != nil {
					return nil, apperr.Newf(apperr.Validation, "Failed to resolve host %q: %v", host, err)
				}
				ips = resolved
			}
			if len(ips) == 0 {
				return nil, apperr.Newf(apperr.Validation, "No IP addresses found for host %q", host)
			}
			for _, ip := range ips {
				if isBlockedIP(ip, mf.AllowLocalhost) {
					return nil, apperr.Newf(apperr.Validation, "Access to restricted IP address %s is blocked", ip.String())
				}
			}
			var firstErr error
			for _, ip := range ips {
				target := net.JoinHostPort(ip.String(), port)
				conn, err := dialer.DialContext(ctx, network, target)
				if err == nil {
					return conn, nil
				}
				if firstErr == nil {
					firstErr = err
				}
			}
			return nil, firstErr
		},
	}

	mf.HTTPClient = &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return validateURL(req.URL, mf.AllowLocalhost)
		},
	}
	return mf
}

func isBlockedIP(ip net.IP, allowLocalhost bool) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4.IsLoopback() {
			return !allowLocalhost
		}
		if ip4.IsPrivate() ||
			ip4.IsLinkLocalUnicast() ||
			ip4.IsLinkLocalMulticast() ||
			ip4.IsMulticast() ||
			ip4.IsUnspecified() {
			return true
		}
		// 0.0.0.0/8, 100.64.0.0/10 (carrier-grade NAT), >= 240 (reserved)
		if ip4[0] == 0 || (ip4[0] == 100 && (ip4[1]&0xc0) == 64) || ip4[0] >= 240 {
			return true
		}
		// Documentation/test ranges: 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24, 198.18.0.0/15
		if (ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2) ||
			(ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100) ||
			(ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113) ||
			(ip4[0] == 198 && (ip4[1]&0xfe) == 18) {
			return true
		}
		return false
	}
	// IPv6
	if ip.IsLoopback() {
		return !allowLocalhost
	}
	if ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}
	return false
}

func validateURL(u *url.URL, allowLocalhost bool) error {
	if u == nil {
		return apperr.New(apperr.Validation, "URL cannot be nil")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return apperr.Newf(apperr.Validation, "Unsupported URL scheme %q; only HTTP and HTTPS are allowed", u.Scheme)
	}
	hostname := u.Hostname()
	if hostname == "" {
		return apperr.New(apperr.Validation, "URL host cannot be empty")
	}
	lowerHost := strings.ToLower(hostname)
	if !allowLocalhost {
		if lowerHost == "localhost" ||
			strings.HasSuffix(lowerHost, ".localhost") ||
			strings.HasSuffix(lowerHost, ".local") ||
			strings.HasSuffix(lowerHost, ".internal") {
			return apperr.Newf(apperr.Validation, "Access to local host %q is blocked", hostname)
		}
	}
	if parsedIP := net.ParseIP(hostname); parsedIP != nil {
		if isBlockedIP(parsedIP, allowLocalhost) {
			return apperr.Newf(apperr.Validation, "Access to restricted IP address %s is blocked", hostname)
		}
	}
	return nil
}

// Fetch downloads media content from rawURL, validating SSRF and enforcing maxSize bytes.
func (mf *MediaFetcher) Fetch(ctx context.Context, rawURL string, maxSize int64) ([]byte, string, string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", "", apperr.Newf(apperr.Validation, "Invalid media URL: %v", err)
	}
	if err := validateURL(parsedURL, mf.AllowLocalhost); err != nil {
		return nil, "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", apperr.Newf(apperr.Validation, "Failed to create request for media URL: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Telesend/1.0; +https://github.com/dsnbyte/telesend)")
	req.Header.Set("Accept", "*/*")

	client := mf.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", apperr.Newf(apperr.Validation, "Failed to fetch media from URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", "", apperr.Newf(apperr.Validation, "Failed to fetch media from URL: HTTP %d", resp.StatusCode)
	}

	if maxSize <= 0 {
		maxSize = mf.MaxMediaSizeBytes
	}

	if resp.ContentLength > maxSize {
		return nil, "", "", apperr.Newf(apperr.Validation, "Media file from URL exceeds maximum allowed size (%d MB)", maxSize/(1024*1024))
	}

	limitReader := io.LimitReader(resp.Body, maxSize+1)
	data, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, "", "", apperr.Newf(apperr.Validation, "Failed to read media content from URL: %v", err)
	}
	if int64(len(data)) > maxSize {
		return nil, "", "", apperr.Newf(apperr.Validation, "Media file from URL exceeds maximum allowed size (%d MB)", maxSize/(1024*1024))
	}

	contentType := resp.Header.Get("Content-Type")
	filename := inferFilename(parsedURL, resp.Header.Get("Content-Disposition"), contentType)

	return data, filename, contentType, nil
}

func inferFilename(u *url.URL, contentDisposition, contentType string) string {
	if contentDisposition != "" {
		if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
			if cdName := params["filename"]; cdName != "" {
				return filepath.Base(cdName)
			}
		}
	}
	if u != nil && u.Path != "" {
		base := filepath.Base(u.Path)
		ext := filepath.Ext(base)
		if ext != "" && base != "." && base != "/" {
			return base
		}
	}
	var ext string
	if contentType != "" {
		mediaType, _, _ := mime.ParseMediaType(contentType)
		switch mediaType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		case "video/mp4":
			ext = ".mp4"
		case "video/webm":
			ext = ".webm"
		case "video/quicktime":
			ext = ".mov"
		case "audio/mpeg":
			ext = ".mp3"
		case "audio/ogg":
			ext = ".ogg"
		case "audio/wav":
			ext = ".wav"
		case "application/pdf":
			ext = ".pdf"
		default:
			if exts, _ := mime.ExtensionsByType(mediaType); len(exts) > 0 {
				ext = exts[0]
			}
		}
	}
	if ext != "" {
		return "media" + ext
	}
	return "media.bin"
}
