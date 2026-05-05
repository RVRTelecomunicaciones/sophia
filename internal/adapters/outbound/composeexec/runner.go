package composeexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Config struct {
	DockerBinary string
	ExtraArgs    []string
}

type Runner struct {
	cfg Config
}

func New(cfg Config) *Runner {
	if cfg.DockerBinary == "" {
		cfg.DockerBinary = "docker"
	}
	return &Runner{cfg: cfg}
}

func (r *Runner) Version(ctx context.Context) (string, error) {
	args := r.cfg.ExtraArgs
	if len(args) == 0 {
		args = []string{"compose", "version"}
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, r.cfg.DockerBinary, args...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("compose version: %w (output: %s)", err, out.String())
	}
	return strings.TrimSpace(out.String()), nil
}

func (r *Runner) Up(ctx context.Context, composePath, projectName string) error {
	if composePath == "" || projectName == "" {
		return errors.New("composePath and projectName are required")
	}
	cmd := exec.CommandContext(ctx, r.cfg.DockerBinary, "compose", "-p", projectName, "-f", composePath, "up", "-d")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose up: %w (output: %s)", err, out)
	}
	return nil
}

func (r *Runner) Down(ctx context.Context, composePath, projectName string) error {
	if composePath == "" || projectName == "" {
		return errors.New("composePath and projectName are required")
	}
	cmd := exec.CommandContext(ctx, r.cfg.DockerBinary, "compose", "-p", projectName, "-f", composePath, "down")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose down: %w (output: %s)", err, out)
	}
	return nil
}
