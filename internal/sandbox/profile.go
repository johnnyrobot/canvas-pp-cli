// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: Canvas Builder's settings, turned into the one object the rest
// of the CLI asks "may I send this request?".
//
// This package is a leaf: it imports only the standard library, so the config
// and the client can both depend on it.

package sandbox

import (
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Settings is what Canvas Builder writes into the workspace's config file.
type Settings struct {
	// Gateway is cb-gateway's address, which must be http://127.0.0.1:<port>.
	Gateway string
	// Workspace is the course workspace folder, an absolute path.
	Workspace string
	// ApprovalTimeout must match cb-gateway's -approval-timeout. Empty means
	// two minutes, the gateway's own default.
	ApprovalTimeout string
	// ChecksCommand runs the bundled checks: the Node binary and cli.ts.
	ChecksCommand []string
}

// Profile is the validated form of Settings. Everything downstream reads it and
// never re-reads the config.
type Profile struct {
	Gateway         *url.URL
	Workspace       string
	ApprovalTimeout time.Duration
	ChecksCommand   []string
}

// defaultApprovalTimeout matches cb-gateway's -approval-timeout default.
const defaultApprovalTimeout = 2 * time.Minute

// New validates Canvas Builder's settings. Every fault is MsgSettings: the
// instructor can't repair this file by hand, and the one thing that helps is
// opening the course from Canvas Builder again. An address the CLI won't use is
// named by NotGateway instead, but only where there is a real gateway to
// compare it against — a gateway setting that isn't the gateway is a damaged
// settings file, not a request pointing somewhere else.
func New(s Settings) (*Profile, error) {
	gateway, err := parseGateway(s.Gateway)
	if err != nil {
		return nil, err
	}
	workspace := strings.TrimSpace(s.Workspace)
	if workspace == "" || !filepath.IsAbs(workspace) {
		return nil, settingsErr()
	}
	wait := defaultApprovalTimeout
	if raw := strings.TrimSpace(s.ApprovalTimeout); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return nil, settingsErr()
		}
		wait = parsed
	}
	return &Profile{
		Gateway:         gateway,
		Workspace:       filepath.Clean(workspace),
		ApprovalTimeout: wait,
		ChecksCommand:   append([]string(nil), s.ChecksCommand...),
	}, nil
}

// Allows reports whether a URL is the gateway: the same scheme, host and port,
// and no user info, which is a place a token could be smuggled. Everything the
// CLI is about to fetch goes through here — the base URL after every override,
// every redirect hop, an upload_url, a file url and every Link next page.
func (p *Profile) Allows(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	if u.Scheme != p.Gateway.Scheme || u.Host != p.Gateway.Host {
		return false
	}
	return u.User == nil
}

// HTTPTimeout keeps the CLI waiting at least as long as the instructor has to
// answer, plus half a minute. The gateway denies a held approval the moment the
// client hangs up, so a CLI that times out first turns a slow approval into a
// denial.
func (p *Profile) HTTPTimeout(flag time.Duration) time.Duration {
	floor := p.ApprovalTimeout + 30*time.Second
	if flag > floor {
		return flag
	}
	return floor
}

// HostPort is the gateway's host:port, such as 127.0.0.1:8181.
func (p *Profile) HostPort() string {
	return p.Gateway.Host
}

// parseGateway holds the gateway address to http://127.0.0.1:<port> and nothing
// else: no https, no host name that could resolve anywhere, no user info, no
// path, query or fragment. The returned URL carries only the scheme and host,
// so printing it can't leak anything.
func parseGateway(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, settingsErr()
	}
	if u.Scheme != "http" || u.Opaque != "" || u.User != nil {
		return nil, settingsErr()
	}
	if u.Hostname() != "127.0.0.1" {
		return nil, settingsErr()
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, settingsErr()
	}
	if u.Path != "" && u.Path != "/" {
		return nil, settingsErr()
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, settingsErr()
	}
	return &url.URL{Scheme: "http", Host: u.Host}, nil
}

func settingsErr() error { return NewError(MsgSettings) }
