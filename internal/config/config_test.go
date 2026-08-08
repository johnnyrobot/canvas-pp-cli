// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for credential resolution.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points config resolution at a temp directory and clears every
// credential env var, so a test never reads the developer's real ~/.config or
// inherits a token from the surrounding shell.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("CANVAS_CONFIG", "")
	t.Setenv("CANVAS_API_TOKEN", "")
	t.Setenv("CANVAS_ACCESS_TOKEN", "")
	t.Setenv("CANVAS_BASE_URL", "")
	return dir
}

// writeConfig writes a config file and points CANVAS_CONFIG at it.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CANVAS_CONFIG", path)
	return path
}

func mustLoad(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

// TestLoad_NoCredentialsAnywhere is the baseline: nothing configured means no
// auth header, not a partially-built one.
func TestLoad_NoCredentialsAnywhere(t *testing.T) {
	isolate(t)
	cfg := mustLoad(t)

	if got := cfg.AuthHeader(); got != "" {
		t.Errorf("AuthHeader with nothing configured = %q, want empty", got)
	}
	if cfg.BaseURL == "" {
		t.Error("BaseURL should keep its default even with no config")
	}
}

// TestLoad_MissingConfigFileIsNotAnError guards the common first-run path.
func TestLoad_MissingConfigFileIsNotAnError(t *testing.T) {
	isolate(t)
	t.Setenv("CANVAS_CONFIG", filepath.Join(t.TempDir(), "does-not-exist.toml"))

	if _, err := Load(""); err != nil {
		t.Errorf("Load with a missing config file = %v, want no error", err)
	}
}

// TestLoad_ReadsTokenFromConfigFile covers the on-disk path.
func TestLoad_ReadsTokenFromConfigFile(t *testing.T) {
	isolate(t)
	writeConfig(t, "api_token = \"from-file\"\n")

	cfg := mustLoad(t)
	if cfg.CanvasApiToken != "from-file" {
		t.Errorf("CanvasApiToken = %q, want %q", cfg.CanvasApiToken, "from-file")
	}
	if !strings.Contains(cfg.AuthHeader(), "from-file") {
		t.Errorf("AuthHeader = %q, want it to carry the file token", cfg.AuthHeader())
	}
}

// TestLoad_EnvBeatsConfigFile pins the headline precedence rule. Env vars are
// applied last in Load, which is what makes them win.
func TestLoad_EnvBeatsConfigFile(t *testing.T) {
	isolate(t)
	writeConfig(t, "api_token = \"from-file\"\naccess_token = \"file-access\"\n")
	t.Setenv("CANVAS_API_TOKEN", "from-env")

	cfg := mustLoad(t)
	if cfg.CanvasApiToken != "from-env" {
		t.Errorf("CanvasApiToken = %q, want the env value to win", cfg.CanvasApiToken)
	}
	if !strings.Contains(cfg.AuthHeader(), "from-env") {
		t.Errorf("AuthHeader = %q, want the env token", cfg.AuthHeader())
	}
	if cfg.AuthSource != "env:CANVAS_API_TOKEN" {
		t.Errorf("AuthSource = %q, want it to name the env var", cfg.AuthSource)
	}
}

// TestLoad_AccessTokenEnvOverridesFile covers the second env var.
func TestLoad_AccessTokenEnvOverridesFile(t *testing.T) {
	isolate(t)
	writeConfig(t, "access_token = \"file-access\"\n")
	t.Setenv("CANVAS_ACCESS_TOKEN", "env-access")

	cfg := mustLoad(t)
	if cfg.AccessToken != "env-access" {
		t.Errorf("AccessToken = %q, want the env value to win", cfg.AccessToken)
	}
	if cfg.AuthSource != "env:CANVAS_ACCESS_TOKEN" {
		t.Errorf("AuthSource = %q", cfg.AuthSource)
	}
}

// TestAuthHeader_ApiTokenBeatsAccessToken pins the field-priority chain, which
// is a second precedence rule layered on top of Load's: even with both tokens
// resolved, only one becomes the Authorization header.
func TestAuthHeader_ApiTokenBeatsAccessToken(t *testing.T) {
	isolate(t)
	t.Setenv("CANVAS_API_TOKEN", "api-tok")
	t.Setenv("CANVAS_ACCESS_TOKEN", "access-tok")

	cfg := mustLoad(t)
	got := cfg.AuthHeader()
	if !strings.Contains(got, "api-tok") {
		t.Errorf("AuthHeader = %q, want the api token to win", got)
	}
	if strings.Contains(got, "access-tok") {
		t.Errorf("AuthHeader = %q, must not carry the access token when an api token exists", got)
	}
}

// TestAuthHeader_ExplicitHeaderBeatsEverything covers the raw-header escape
// hatch, which short-circuits both tokens.
func TestAuthHeader_ExplicitHeaderBeatsEverything(t *testing.T) {
	isolate(t)
	writeConfig(t, "auth_header = \"Bearer literal-header\"\n")
	t.Setenv("CANVAS_API_TOKEN", "api-tok")

	cfg := mustLoad(t)
	if got := cfg.AuthHeader(); got != "Bearer literal-header" {
		t.Errorf("AuthHeader = %q, want the literal auth_header", got)
	}
}

// TestAuthHeader_FormatsAsBearer pins the wire format.
func TestAuthHeader_FormatsAsBearer(t *testing.T) {
	isolate(t)
	t.Setenv("CANVAS_API_TOKEN", "tok123")

	if got := mustLoad(t).AuthHeader(); got != "Bearer tok123" {
		t.Errorf("AuthHeader = %q, want %q", got, "Bearer tok123")
	}
}

// TestAuthHeader_AccessTokenAloneStillAuthenticates covers the OAuth path,
// where only an access token is present.
func TestAuthHeader_AccessTokenAloneStillAuthenticates(t *testing.T) {
	isolate(t)
	t.Setenv("CANVAS_ACCESS_TOKEN", "acc456")

	if got := mustLoad(t).AuthHeader(); got != "Bearer acc456" {
		t.Errorf("AuthHeader = %q, want %q", got, "Bearer acc456")
	}
}

// TestLoad_BaseURLOverride covers the env override used to point the CLI at a
// self-hosted Canvas or a test server.
func TestLoad_BaseURLOverride(t *testing.T) {
	isolate(t)
	writeConfig(t, "base_url = \"https://from-file.example\"\n")

	if got := mustLoad(t).BaseURL; got != "https://from-file.example" {
		t.Errorf("BaseURL from file = %q", got)
	}

	t.Setenv("CANVAS_BASE_URL", "https://from-env.example")
	if got := mustLoad(t).BaseURL; got != "https://from-env.example" {
		t.Errorf("BaseURL = %q, want the env override to win", got)
	}
}

// TestLoad_ExplicitPathBeatsEnv covers the --config flag outranking
// CANVAS_CONFIG.
func TestLoad_ExplicitPathBeatsEnv(t *testing.T) {
	isolate(t)
	writeConfig(t, "api_token = \"via-env-var\"\n")

	explicit := filepath.Join(t.TempDir(), "explicit.toml")
	if err := os.WriteFile(explicit, []byte("api_token = \"via-explicit-path\"\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := Load(explicit)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CanvasApiToken != "via-explicit-path" {
		t.Errorf("CanvasApiToken = %q, want the explicitly passed file to win", cfg.CanvasApiToken)
	}
}

// TestLoad_ReportsResolvedPath keeps `doctor` able to say where config came
// from.
func TestLoad_ReportsResolvedPath(t *testing.T) {
	isolate(t)
	path := writeConfig(t, "api_token = \"x\"\n")

	if got := mustLoad(t).Path; got != path {
		t.Errorf("Path = %q, want %q", got, path)
	}
}

// TestLoad_MalformedConfigIsAnError keeps a broken file from silently
// resolving to no credentials, which would surface as a confusing 401.
func TestLoad_MalformedConfigIsAnError(t *testing.T) {
	isolate(t)
	writeConfig(t, "this is not = valid = toml [[[\n")

	if _, err := Load(""); err == nil {
		t.Error("Load with malformed TOML = nil error, want a parse error")
	}
}
