package bkbuilder

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/moby/buildkit/client"
	"github.com/tonistiigi/fsutil"
	"google.golang.org/grpc"
)

// BkBuilder holds a global parameters that used for each build process
type BkBuilder struct {
	// Call LoadDefaultConfigFile(os.Stderr) from "github.com/docker/cli/cli/config" package
	// to support private registries
	// authProvider session.Attachable
	// holds buildkit client.
	Client *client.Client
}

// the New function initial BkBuilder and return reference to the BkBuilder.
// BkBuilder members that are initialized in the function:
// 1. Client, initial with moby/buildkit client, Used DEDICATED_IMAGE_BUILDING_ADDRESS to connect to buildkit daemon, default 'tcp://localhost:1234'
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

	// ========= REMOVE MY AFTER THE authProvider IS UNCOMMENTED
	// before turring on the authProvider feature, I need to investigate what we want do to with the paramter the passed to LoadDefaultConfigFile function
	// =========
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

// Run build and push(always) from dockerfile.
// the function return reference to BuildResult that caller can used with.
// if dockerFilename paramater is empty the default 'Dockerfile' is used.
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

	result := &BuildResult{
		ImageUrl: imageUrl,
	}
	statusChan := make(chan *client.SolveStatus)
	go result.updateStatus(statusChan)
	go func() {
		result.updateSolveResult(bk.Client.Solve(ctx, nil, solveOpt, statusChan))
	}()
	return result, nil
}
