package bkbuilder

import (
	"context"
	"log/slog"
	"os"

	"github.com/docker/cli/cli/config"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
)

type BkBuilder struct {
	authProvider session.Attachable
	Client       *client.Client
}

func NewBkBuilder(ctx context.Context, buildkitDaemon string) *BkBuilder {

	bkClient, err := client.New(ctx, buildkitDaemon)
	if err != nil {
		slog.Error("failed to connect to buildkit", slog.String("error", err.Error()))
	}

	cfg := config.LoadDefaultConfigFile(os.Stderr)
	BkBuilder := BkBuilder{
		authProvider: authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{ConfigFile: cfg}),
		Client:       bkClient,
	}

	return &BkBuilder
}

func (bkBuilder BkBuilder) Close() {
	bkBuilder.Client.Close()
}
