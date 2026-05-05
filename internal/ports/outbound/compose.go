package outbound

import "context"

type ComposeRunner interface {
	Version(ctx context.Context) (string, error)
	Up(ctx context.Context, composePath, projectName string) error
	Down(ctx context.Context, composePath, projectName string) error
}
