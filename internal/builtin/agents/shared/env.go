package shared

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/peggco/pegg/internal/agent/prompt"
	"github.com/peggco/pegg/internal/core/config"
)

const cmdTimeout = 3 * time.Second

func sharedVars(providerID, modelID string) prompt.Vars {
	model := modelID
	if model == "" {
		model = "unknown"
	}
	v := prompt.Vars{
		"name":  config.AppName,
		"model": model,
	}
	for k, val := range envVars() {
		v[k] = val
	}
	for k, val := range gitVars() {
		v[k] = val
	}
	return v
}

func envVars() prompt.Vars {
	wd, _ := os.Getwd()
	return prompt.Vars{
		"env": map[string]any{
			"Working_directory": wd,
			"platform":          runtime.GOOS,
			"os_version":        osVersion(),
			"date":              time.Now().Format("Mon Jan 02 2006"),
			"shell":             shellName(),
		},
	}
}

func shellName() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
}

func osVersion() string {
	switch runtime.GOOS {
	case "linux":
		if pretty := osReleasePretty(); pretty != "" {
			return pretty
		}
	case "darwin":
		if v := runCmd("sw_vers", "-productVersion"); v != "" {
			return "macOS " + v
		}
	case "windows":
		if v := runCmd("cmd", "/c", "ver"); v != "" {
			return v
		}
	default:
		if v := runCmd("uname", "-sr"); v != "" {
			return v
		}
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}

func osReleasePretty() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return ""
}

func gitVars() prompt.Vars {
	vars := prompt.Vars{
		"git": map[string]any{
			"enabled":        false,
			"branch":         "",
			"main_branch":    "",
			"status":         "",
			"recent_commits": "",
		},
	}
	if runCmd("git", "rev-parse", "--is-inside-work-tree") != "true" {
		return vars
	}
	git := vars["git"].(map[string]any)
	git["enabled"] = true
	git["branch"] = runCmd("git", "branch", "--show-current")
	git["main_branch"] = gitMainBranch()
	git["status"] = runCmd("git", "status", "--short")
	git["recent_commits"] = runCmd("git", "log", "--oneline", "-5")
	return vars
}

func gitMainBranch() string {
	if ref := runCmd("git", "symbolic-ref", "refs/remotes/origin/HEAD"); ref != "" {
		parts := strings.Split(ref, "/")
		if last := parts[len(parts)-1]; last != "" {
			return last
		}
	}
	return "main"
}

func runCmd(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
