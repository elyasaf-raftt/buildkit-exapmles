package bkbuilder

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/docker/cli/cli/config"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/tonistiigi/fsutil"
	"google.golang.org/grpc"
)

// Interface BuildOptions use to support docker build attributes, such as build-arg and no-cache.
//
// supported attributes: type BuildArgs, type NoCache
type BuildOptions interface {
	frontendAttrs(map[string]string)
}

// BuildArgs implement BuildOptions to support build-arg attribute
// its equivalent to --build-arg from docker build cli.
type BuildArgs map[string]string

func (ba BuildArgs) frontendAttrs(attrs map[string]string) {
	for k, v := range ba {
		attrs[fmt.Sprintf("build-arg:%s", k)] = v
	}
}

// NoCache implement BuildOptions to support no-cache attribute,
// its equivalent to --no-cache from docker build cli.
type NoCache bool

func (nc NoCache) frontendAttrs(attrs map[string]string) {
	if nc {
		attrs["no-cache"] = ""
	} else {
		attrs["no-cache"] = "false"
	}
}

// BkBuilder holds a global parameters used for all build processes.
type BkBuilder struct {
	// authProvider manages the private registries configurations,
	//
	// the private registry configuration is loaded using LoadDefaultConfigFile(io.Writer) from the "github.com/docker/cli/cli/config" package.
	// make sure your private registry configurations exists at least in one of the default docker configuration files, such as ~/.docker/config.json
	//
	// hold Attachable object that return from moby/buildkit/session/auth/authprovider NewDockerAuthProvider function
	authProvider session.Attachable
	// holds buildkit client.
	Client *client.Client
}

// the New function initial BkBuilder and return reference to the BkBuilder.
//
// BkBuilder members that are initialized in the function:
//
// 1. Client, initial with moby/buildkit client, used DEDICATED_IMAGE_BUILDING_ADDRESS to connect to buildkit daemon, default 'tcp://localhost:1234'.
//
// 2. authProvider optional, it is initialized if there is any docker configuration
// loaded with LoadDefaultConfigFile(io.Writer) function from the "github.com/docker/cli/cli/config" package.
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

	BkBuilder := BkBuilder{
		Client: bkClient,
	}

	dockerFileWarning := bytes.Buffer{}
	cfg := config.LoadDefaultConfigFile(&dockerFileWarning)
	if dockerFileWarning.Len() != 0 {
		slog.Warn("auth provider failed", slog.String("load configuration failed", dockerFileWarning.String()))
	}
	// running LoadDefaultConfigFile function `it initializes a default ConfigFile struct`.
	// therefore we need to manually check if any configuration is loaded.
	if len(cfg.AuthConfigs) != 0 || cfg.CredentialsStore != "" {
		BkBuilder.authProvider = authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{ConfigFile: cfg})
	}

	return &BkBuilder, nil
}

func (bkBuilder BkBuilder) Close() {
	bkBuilder.Client.Close()
}

// Run build and push(always) from dockerfile.
// the function return reference to BuildResult.
//
// if dockerFilename paramater is empty the default 'Dockerfile' is used.
func (bk *BkBuilder) BuildFromDockerfile(ctx context.Context, folderPath string, dockerFilename string, imageUrl string, buildOptions ...BuildOptions) (*BuildResult, error) {

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

	for _, opt := range buildOptions {
		opt.frontendAttrs(solveOpt.FrontendAttrs)
	}

	if bk.authProvider != nil {
		solveOpt.Session = append(solveOpt.Session, bk.authProvider)
	}

	result := &BuildResult{
		ImageUrl: imageUrl,
		done:     make(chan any),
	}

	statusChan := make(chan *client.SolveStatus)

	go result.updateStatus(statusChan)
	go func() {
		res, err := bk.Client.Solve(ctx, nil, solveOpt, statusChan)
		result.updateSolveResult(res, err)
	}()
	return result, nil
}
