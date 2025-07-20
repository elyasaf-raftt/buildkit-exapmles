package buildkitconfig

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/tonistiigi/fsutil"
)

type Config struct {
	Ctx              context.Context
	Daemon           string
	RegistryUrl      string
	DockerfilePath   string
	ImageName        string
	Image            string
	DockerConfigFile *configfile.ConfigFile
}

func GetConfig() (Config, error) {
	ctx := context.Background()

	args := os.Args[1:] // Skip the program name

	params := make(map[string]string)
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]
			params[key] = value
		}
	}

	buildkitDaemon := os.Getenv("BUILDKIT_DAEMON")
	if params["buildkit-daemon"] != "" {
		buildkitDaemon = params["buildkit-daemon"]
	}
	if buildkitDaemon == "" {
		return Config{}, fmt.Errorf("missing buildkit deamon set it with env BUILDKIT_DAEMON or pass it as args buildkit-daemon=<buildkit_daemon>")
	}

	registryUrl := os.Getenv("REGISTRY_URL")
	if params["registry-url"] != "" {
		registryUrl = params["registry-url"]
	}
	if registryUrl == "" {
		return Config{}, fmt.Errorf("missing registryUrl set it with env REGISTRY_URL or pass it as args registry-url=<registry-url>")
	}

	dockerfilePath := os.Getenv("DOCKERFILE_PATH")
	if params["dockerfile-path"] != "" {
		dockerfilePath = params["dockerfile-path"]
	}
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
		slog.Info("uses default: dockerfile ./Dockerfile")

	}

	ImageName := os.Getenv("IMAGE_NAME")
	if params["image-name"] != "" {
		ImageName = params["image-name"]
	}
	if ImageName == "" {
		ImageName = "test"
		slog.Info("used default: imageName test")
	}

	config := Config{
		Ctx:            ctx,
		Daemon:         buildkitDaemon,
		RegistryUrl:    registryUrl,
		DockerfilePath: dockerfilePath,
		Image:          fmt.Sprintf("%s/%s", registryUrl, strings.TrimPrefix(ImageName, "/")),
	}

	username := os.Getenv("USERNAME")
	if params["username"] != "" {
		username = params["username"]
	}
	password := os.Getenv("PASSWORD")
	if params["password"] != "" {
		password = params["password"]
	}

	if username != "" && password != "" {
		config.DockerConfigFile = &configfile.ConfigFile{
			AuthConfigs: map[string]types.AuthConfig{
				config.RegistryUrl: {
					Username:      username,
					Password:      password,
					ServerAddress: config.RegistryUrl,
				},
			},
		}
	}

	return config, nil
}

func GetSolveOptForDockerfile(buildkitConfig Config) (client.SolveOpt, error) {

	ctxFS, err := fsutil.NewFS(".")
	if err != nil {
		return client.SolveOpt{}, fmt.Errorf("failed to create FS for context %+w", err)
	}

	opt := client.SolveOpt{
		Frontend: "dockerfile.v0",
		LocalMounts: map[string]fsutil.FS{
			"context":    ctxFS,
			"dockerfile": ctxFS,
		},

		FrontendAttrs: map[string]string{
			"filename": buildkitConfig.DockerfilePath,
		},
		Exports: []client.ExportEntry{
			{
				Type: "image",
				Attrs: map[string]string{
					"name":              buildkitConfig.Image,
					"push":              "true",
					"compression-level": "1",
					"registry.insecure": "true",
				},
			},
		},
	}

	sess, err := session.NewSession(buildkitConfig.Ctx, "build from docker file")
	if err != nil {
		return client.SolveOpt{}, fmt.Errorf("failed to extablish builkdit clinet session %+w", err)
	}
	if buildkitConfig.DockerConfigFile != nil {
		authProvider := authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{
			ConfigFile: buildkitConfig.DockerConfigFile,
		})
		sess.Allow(authProvider)
		opt.Session = append(opt.Session, authProvider)
	}

	return opt, nil
}
