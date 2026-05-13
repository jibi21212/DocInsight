package crawler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// CrawlOptions configures a crawl run.
type CrawlOptions struct {
	StartURL  string
	MaxDepth  int
	MaxPages  int
	UserAgent string
}

// CrawlResult holds all discovered URLs from the crawl.
type CrawlResult struct {
	URLs []string
}

// Crawler performs BFS web crawling within a single domain.
type Crawler struct {
	client *http.Client
}

// NewCrawler creates a crawler with the given timeout.
func NewCrawler(timeoutSec int) *Crawler {
	return &Crawler{
		client: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
	}
}

// Crawl performs a BFS traversal from opts.StartURL, discovering links
// on the same domain up to MaxDepth hops and MaxPages total.
func (c *Crawler) Crawl(ctx context.Context, opts CrawlOptions) (*CrawlResult, error) {
	startParsed, err := url.Parse(opts.StartURL)
	if err != nil {
		return nil, err
	}

	domain := startParsed.Host
	visited := make(map[string]bool)
	var collected []string

	type queueItem struct {
		url   string
		depth int
	}
	queue := []queueItem{{url: opts.StartURL, depth: 0}}

	for len(queue) > 0 && len(collected) < opts.MaxPages {
		select {
		case <-ctx.Done():
			return &CrawlResult{URLs: collected}, ctx.Err()
		default:
		}

		item := queue[0]
		queue = queue[1:]

		normalized := normalizeURL(item.url)
		if visited[normalized] {
			continue
		}
		visited[normalized] = true
		collected = append(collected, item.url)

		if item.depth >= opts.MaxDepth {
			continue
		}

		links, err := c.fetchLinks(ctx, item.url, opts.UserAgent)
		if err != nil {
			slog.Debug("crawler: failed to fetch", "url", item.url, "error", err)
			continue
		}

		for _, link := range links {
			linkParsed, err := url.Parse(link)
			if err != nil {
				continue
			}

			// Resolve relative URLs
			resolved := startParsed.ResolveReference(linkParsed)

			// Same domain only
			if resolved.Host != domain {
				continue
			}

			// Only http/https
			if resolved.Scheme != "http" && resolved.Scheme != "https" {
				continue
			}

			// Skip anchors, common non-content paths
			if shouldSkipURL(resolved) {
				continue
			}

			resolvedStr := resolved.String()
			norm := normalizeURL(resolvedStr)
			if !visited[norm] && len(collected)+len(queue) < opts.MaxPages*2 {
				queue = append(queue, queueItem{url: resolvedStr, depth: item.depth + 1})
			}
		}
	}

	return &CrawlResult{URLs: collected}, nil
}

func (c *Crawler) fetchLinks(ctx context.Context, pageURL string, userAgent string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		return nil, nil
	}

	// Limit body read to 2MB to avoid downloading huge pages
	limited := io.LimitReader(resp.Body, 2*1024*1024)
	return extractLinks(limited)
}

func extractLinks(r io.Reader) ([]string, error) {
	tokenizer := html.NewTokenizer(r)
	var links []string

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return links, nil
			}
			return links, tokenizer.Err()
		case html.StartTagToken, html.SelfClosingTagToken:
			t := tokenizer.Token()
			if t.Data == "a" {
				for _, attr := range t.Attr {
					if attr.Key == "href" {
						href := strings.TrimSpace(attr.Val)
						if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") && !strings.HasPrefix(href, "mailto:") {
							links = append(links, href)
						}
					}
				}
			}
		}
	}
}

func normalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// Remove fragment
	u.Fragment = ""
	// Remove trailing slash for consistency
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}

func shouldSkipURL(u *url.URL) bool {
	path := strings.ToLower(u.Path)
	skipSuffixes := []string{
		".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg",
		".ico", ".woff", ".woff2", ".ttf", ".eot", ".mp4", ".mp3",
		".pdf", ".zip", ".tar", ".gz", ".xml", ".json",
	}
	for _, s := range skipSuffixes {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}
