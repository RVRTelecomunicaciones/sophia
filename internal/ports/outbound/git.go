package outbound

import "context"

type GitInspector interface {
	Version(ctx context.Context) (string, error)
	RepoRoot(ctx context.Context, cwd string) (string, error)
	RemoteURL(ctx context.Context, cwd string) (string, error)
	CurrentBranch(ctx context.Context, cwd string) (string, error)
	Status(ctx context.Context, cwd string) (clean bool, files int, err error)
}
