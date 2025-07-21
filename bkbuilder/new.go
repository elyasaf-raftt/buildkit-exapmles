package bkbuilder

import (
	"context"
	"os"

	"github.com/docker/cli/cli/config"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
)

type BkBuilder struct {
	authProvider session.Attachable
}

func NewBkBuilder(ctx context.Context, buildkitDaemon string) *BkBuilder {

	cfg := config.LoadDefaultConfigFile(os.Stderr)
	BkBuilder := BkBuilder{
		authProvider: authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{ConfigFile: cfg}),
	}

	return &BkBuilder
}
