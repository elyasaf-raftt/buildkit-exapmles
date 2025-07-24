package bkbuilder

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/tonistiigi/fsutil"
	"google.golang.org/grpc"
)

type BkBuilder struct {
	authProvider session.Attachable
	Client       *client.Client
}

func New(ctx context.Context) (*BkBuilder, error) {

	buildkitClientOpts := []client.ClientOpt{
		client.WithGRPCDialOption(grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: time.Second * 5,
		})),
	}

	bkUrl := os.Getenv("DEDICATED_IMAGE_BUILDING_ADDRESS")
	if bkUrl == "" {
		slog.Warn("environemnt variable DEDICATED_IMAGE_BUILDING_ADDRESS is not set, using buildkit defualt address")
		bkUrl = "tcp://localhost:1234"
	}
	bkClient, err := client.New(ctx, bkUrl, buildkitClientOpts...)
	if err != nil {
		return nil, fmt.Errorf("falied to initilaize buildkit client: %+w", err)
	}

	// cfg := config.LoadDefaultConfigFile(os.Stderr)
	BkBuilder := BkBuilder{
		// authProvider: authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{ConfigFile: cfg}),
		Client: bkClient,
	}
	return &BkBuilder, nil
}

func (bkBuilder BkBuilder) Close() {
	bkBuilder.Client.Close()
}

func (bk *BkBuilder) BuildFromDockerfile(ctx context.Context, folderPath string, dockerFilename string, imageUrl string) (*BuildResult, error) {

	buildContext, err := fsutil.NewFS(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read docker context directory: %w", err)
	}

	if dockerFilename == "" {
		dockerFilename = "Dockerfile"
	}

	solveOpt := client.SolveOpt{
		Frontend: "dockerfile.v0",
		LocalMounts: map[string]fsutil.FS{
			"context":    buildContext,
			"dockerfile": buildContext,
		},

		FrontendAttrs: map[string]string{
			"filename": dockerFilename,
		},
		Exports: []client.ExportEntry{
			{
				Type: "image",
				Attrs: map[string]string{
					"name":              imageUrl,
					"push":              "true",
					"compression-level": "1",
					"registry.insecure": "true",
				},
			},
		},
	}

	result := &BuildResult{}
	statusChan := make(chan *client.SolveStatus)
	go result.updateStatus(statusChan)
	go func() {
		result.updateSolveResult(bk.Client.Solve(ctx, nil, solveOpt, statusChan))
	}()
	return result, nil
}
