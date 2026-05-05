package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Config struct {
	Binary      string
	VersionArgs []string
}

type Inspector struct {
	cfg Config
}

func New(cfg Config) *Inspector {
	if cfg.Binary == "" {
		cfg.Binary = "git"
	}
	return &Inspector{cfg: cfg}
}

func (g *Inspector) run(ctx context.Context, cwd string, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, g.cfg.Binary, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w (output: %s)", args, err, out.String())
	}
	return strings.TrimSpace(out.String()), nil
}

func (g *Inspector) Version(ctx context.Context) (string, error) {
	args := g.cfg.VersionArgs
	if len(args) == 0 {
		args = []string{"--version"}
	}
	return g.run(ctx, "", args...)
}

func (g *Inspector) RepoRoot(ctx context.Context, cwd string) (string, error) {
	return g.run(ctx, cwd, "rev-parse", "--show-toplevel")
}

func (g *Inspector) RemoteURL(ctx context.Context, cwd string) (string, error) {
	out, err := g.run(ctx, cwd, "config", "--get", "remote.origin.url")
	if err != nil {
		return "", nil
	}
	return out, nil
}

func (g *Inspector) CurrentBranch(ctx context.Context, cwd string) (string, error) {
	return g.run(ctx, cwd, "symbolic-ref", "--short", "HEAD")
}

func (g *Inspector) Status(ctx context.Context, cwd string) (bool, int, error) {
	out, err := g.run(ctx, cwd, "status", "--porcelain")
	if err != nil {
		return false, 0, err
	}
	if out == "" {
		return true, 0, nil
	}
	lines := strings.Split(out, "\n")
	return false, len(lines), nil
}
