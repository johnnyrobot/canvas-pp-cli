// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the Canvas Builder sandbox settings and the
// fail-closed config load.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

const sandboxConfig = `sandbox = true
base_url = "http://127.0.0.1:8181"
gateway = "http://127.0.0.1:8181"
workspace = "/Users/someone/Canvas Builder/Intro to Art"
approval_timeout = "120s"
checks_command = ["/opt/cb/node", "/opt/cb/checks/src/cli.ts"]
`

// cliHome lays out a course workspace's CLI home the way the app writes it:
// config/config.toml with the sandbox settings and data/credentials.toml with
// the agent token. Every other way of finding a config or a token is cleared,
// so a test never reads the developer's own.
func cliHome(t *testing.T, configBody string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", filepath.Join(home, "fake-home"))
	t.Setenv("CANVAS_HOME", home)
	for _, name := range []string{
		"CANVAS_CONFIG", "CANVAS_CONFIG_DIR", "CANVAS_DATA_DIR", "CANVAS_STATE_DIR", "CANVAS_CACHE_DIR",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME",
		"CANVAS_API_TOKEN", "CANVAS_ACCESS_TOKEN", "CANVAS_BASE_URL",
	} {
		t.Setenv(name, "")
	}
	if configBody != "" {
		write(t, filepath.Join(home, "config", "config.toml"), configBody)
	}
	write(t, filepath.Join(home, "data", "credentials.toml"), "api_token = \"tA\"\n")
	return home
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoad_SandboxConfig(t *testing.T) {
	cliHome(t, sandboxConfig)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Sandbox {
		t.Error("Sandbox = false, want true")
	}
	if cfg.Profile == nil {
		t.Fatal("Profile is nil")
	}
	if got := cfg.Profile.HostPort(); got != "127.0.0.1:8181" {
		t.Errorf("Profile.HostPort() = %q", got)
	}
	if cfg.Profile.Workspace != "/Users/someone/Canvas Builder/Intro to Art" {
		t.Errorf("Profile.Workspace = %q", cfg.Profile.Workspace)
	}
	if cfg.Profile.ApprovalTimeout != 2*time.Minute {
		t.Errorf("Profile.ApprovalTimeout = %v, want 2m", cfg.Profile.ApprovalTimeout)
	}
	if len(cfg.Profile.ChecksCommand) != 2 || cfg.Profile.ChecksCommand[0] != "/opt/cb/node" {
		t.Errorf("Profile.ChecksCommand = %v", cfg.Profile.ChecksCommand)
	}
	if cfg.CanvasApiToken != "tA" {
		t.Errorf("CanvasApiToken = %q, want the token from credentials.toml", cfg.CanvasApiToken)
	}
	if cfg.BaseURL != "http://127.0.0.1:8181" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
}

// TestLoad_SandboxRefusesOtherBaseURL [Review Focus 3]: an agent that points
// the CLI at a real Canvas gets a refusal, not a request carrying its token.
func TestLoad_SandboxRefusesOtherBaseURL(t *testing.T) {
	t.Run("env override", func(t *testing.T) {
		cliHome(t, sandboxConfig)
		t.Setenv("CANVAS_BASE_URL", "https://college.instructure.com")

		_, err := Load("")
		if err == nil {
			t.Fatal("Load accepted CANVAS_BASE_URL pointing off the gateway")
		}
		if !strings.Contains(err.Error(), "https://college.instructure.com") {
			t.Errorf("Load = %q, want the refused address named", err.Error())
		}
		if !strings.Contains(err.Error(), "only talks to Canvas Builder at http://127.0.0.1:8181") {
			t.Errorf("Load = %q, want the NotGateway message", err.Error())
		}
	})

	t.Run("base_url in the file", func(t *testing.T) {
		cliHome(t, strings.Replace(sandboxConfig,
			`base_url = "http://127.0.0.1:8181"`,
			`base_url = "https://college.instructure.com"`, 1))

		_, err := Load("")
		if err == nil {
			t.Fatal("Load accepted a base_url off the gateway")
		}
		if !strings.Contains(err.Error(), "https://college.instructure.com") {
			t.Errorf("Load = %q, want the refused address named", err.Error())
		}
	})
}

func TestLoad_SandboxRefusesExtraHeaders(t *testing.T) {
	t.Run("headers table", func(t *testing.T) {
		cliHome(t, sandboxConfig+"\n[headers]\nX-Forwarded-Host = \"college.instructure.com\"\n")

		_, err := Load("")
		if err == nil || err.Error() != sandbox.MsgExtraHeaders {
			t.Fatalf("Load = %v, want MsgExtraHeaders", err)
		}
	})

	// auth_header in the credentials file, which is where it survives: the
	// credentials file wins over the config file's copy of the same field.
	t.Run("auth_header", func(t *testing.T) {
		home := cliHome(t, sandboxConfig)
		write(t, filepath.Join(home, "data", "credentials.toml"),
			"api_token = \"tA\"\nauth_header = \"Bearer someone-elses-token\"\n")

		_, err := Load("")
		if err == nil || err.Error() != sandbox.MsgExtraHeaders {
			t.Fatalf("Load = %v, want MsgExtraHeaders", err)
		}
	})
}

// TestLoad_StampedBuildNeedsConfig [Review Focus 3]: the bundled build has no
// fallback. No settings, or settings that aren't Canvas Builder's, and it stops.
func TestLoad_StampedBuildNeedsConfig(t *testing.T) {
	t.Run("no config at all", func(t *testing.T) {
		home := cliHome(t, "")
		restore := sandbox.SetRequiredForTesting(true)
		defer restore()
		// A legacy config with a real Canvas in it is exactly what the stamped
		// build must not fall back to.
		write(t, filepath.Join(home, "fake-home", ".config", "canvas-pp-cli", "config.toml"),
			"base_url = \"https://college.instructure.com\"\napi_token = \"not-ours\"\n")

		_, err := Load("")
		if err == nil || err.Error() != sandbox.MsgSettings {
			t.Fatalf("Load = %v, want exactly MsgSettings", err)
		}
	})

	t.Run("config without sandbox", func(t *testing.T) {
		cliHome(t, "base_url = \"http://127.0.0.1:8181\"\n")
		restore := sandbox.SetRequiredForTesting(true)
		defer restore()

		_, err := Load("")
		if err == nil || err.Error() != sandbox.MsgSettings {
			t.Fatalf("Load = %v, want exactly MsgSettings", err)
		}
	})

	t.Run("damaged config", func(t *testing.T) {
		cliHome(t, "sandbox = true\ngateway = \n")
		restore := sandbox.SetRequiredForTesting(true)
		defer restore()

		_, err := Load("")
		if err == nil || err.Error() != sandbox.MsgSettings {
			t.Fatalf("Load = %v, want exactly MsgSettings", err)
		}
	})
}

// TestLoad_NormalModeUnchanged: without the sandbox key nothing about this CLI
// changes, which is what keeps it usable outside Canvas Builder.
func TestLoad_NormalModeUnchanged(t *testing.T) {
	isolate(t)

	cfg := mustLoad(t)
	if cfg.Profile != nil {
		t.Error("Profile is set without sandbox = true")
	}
	if cfg.Sandbox {
		t.Error("Sandbox = true without the key")
	}
	if cfg.BaseURL != "https://canvas.instructure.com" {
		t.Errorf("BaseURL = %q, want the default", cfg.BaseURL)
	}
}

// TestLoad_NormalModeKeepsHeaders: MsgExtraHeaders is a sandbox rule only.
func TestLoad_NormalModeKeepsHeaders(t *testing.T) {
	isolate(t)
	writeConfig(t, "[headers]\nX-Custom = \"yes\"\n")

	cfg := mustLoad(t)
	if cfg.Headers["X-Custom"] != "yes" {
		t.Errorf("Headers = %v, want the header kept outside Canvas Builder", cfg.Headers)
	}
}

// TestSave_KeepsSandboxKeys: a token save must not strip the settings the app
// wrote, or the next run of the CLI fails closed for no reason.
func TestSave_KeepsSandboxKeys(t *testing.T) {
	home := cliHome(t, sandboxConfig)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.SaveTokens("cid", "secret", "atoken", "rtoken", time.Now()); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "config.toml"))
	if err != nil {
		t.Fatalf("reread config: %v", err)
	}
	for _, key := range []string{"sandbox", "gateway", "workspace", "approval_timeout", "checks_command"} {
		if !strings.Contains(string(data), key) {
			t.Errorf("saved config lost %q:\n%s", key, data)
		}
	}

	reloaded, err := Load("")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Profile == nil {
		t.Error("the reloaded config has no Profile")
	}
}

// TestSave_NormalModeOmitsSandboxKeys: the keys are omitempty, so an ordinary
// user's config file is unchanged.
func TestSave_NormalModeOmitsSandboxKeys(t *testing.T) {
	isolate(t)
	path := writeConfig(t, "base_url = \"https://college.instructure.com\"\n")

	cfg := mustLoad(t)
	if err := cfg.SaveTokens("cid", "secret", "atoken", "rtoken", time.Now()); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reread config: %v", err)
	}
	for _, key := range []string{"sandbox", "gateway", "workspace", "approval_timeout", "checks_command"} {
		if strings.Contains(string(data), key) {
			t.Errorf("saved config gained %q:\n%s", key, data)
		}
	}
}

// TestSandboxRequested reports the key without validating anything, so the
// command surface can narrow itself before Load has a chance to fail.
func TestSandboxRequested(t *testing.T) {
	cliHome(t, sandboxConfig)
	if !SandboxRequested("") {
		t.Error("SandboxRequested = false for a sandbox config")
	}

	cliHome(t, "base_url = \"http://127.0.0.1:8181\"\n")
	if SandboxRequested("") {
		t.Error("SandboxRequested = true without the key")
	}

	cliHome(t, "")
	if SandboxRequested("") {
		t.Error("SandboxRequested = true with no config file")
	}

	cliHome(t, "sandbox = true\ngateway = \n")
	if SandboxRequested("") {
		t.Error("SandboxRequested = true for a damaged config")
	}
}
