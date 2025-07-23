package bkbuilder

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/docker/cli/cli/config"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"google.golang.org/grpc"
)

type BkBuilder struct {
	authProvider session.Attachable

	Client *client.Client
}

func NewBkBuilder(ctx context.Context, buildkitDaemon string) *BkBuilder {

	bkClientOpts := []client.ClientOpt{
		client.WithGRPCDialOption(grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: time.Second * 5,
		})),
	}
	bkClient, err := client.New(ctx, buildkitDaemon, bkClientOpts...)
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
