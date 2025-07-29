package buildkitconfig

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
)

type Config struct {
	Ctx              context.Context
	Daemon           string
	RegistryUrl      string
	DockerfileName   string
	ImageUrl         string
	ImageName        string
	DockerConfigFile *configfile.ConfigFile
	FolderPath       string
}

func ParseArgs() (Config, error) {
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

	buildkitDaemon := os.Getenv("DEDICATED_IMAGE_BUILDING_ADDRESS")
	if params["buildkit-daemon"] != "" {
		buildkitDaemon = params["buildkit-daemon"]
	}
	if buildkitDaemon == "" {
		buildkitDaemon = "tcp://localhost:1234"
		slog.Info("use default: buildkit Daemon tcp://localhost:1234")
		// return Config{}, fmt.Errorf("missing buildkit deamon set it with env BUILDKIT_DAEMON or pass it as args buildkit-daemon=<buildkit_daemon>")
	}

	registryUrl := os.Getenv("REGISTRY_URL")
	if params["registry-url"] != "" {
		registryUrl = params["registry-url"]
	}
	if registryUrl == "" {
		// registryUrl = "raftt-image-registry.raftt.svc.cluster.local:80"
		registryUrl = "localhost:5000"
		slog.Info("use default: registry Url", slog.String("registry-url", registryUrl))
		// return Config{}, fmt.Errorf("missing registryUrl set it with env REGISTRY_URL or pass it as args registry-url=<registry-url>")
	}

	dockerfileName := os.Getenv("DOCKERFILE_NAME")
	if params["dockerfile-name"] != "" {
		dockerfileName = params["dockerfile-name"]
	}
	if dockerfileName == "" {
		dockerfileName = "Dockerfile-test"
		slog.Info("use default: dockerfile Dockerfile")
	}

	folderPath := os.Getenv("FOLDER_PATH")
	if params["folder-path"] != "" {
		folderPath = params["folder-path"]
	}
	if folderPath == "" {
		dir, err := os.Getwd()
		if err != nil {
			return Config{}, fmt.Errorf("missing buildkit folder path: failed to get the current path set it with env  FOLDER_PATH or pass it as args folder-path=<folder_path> %+w", err)
		}
		folderPath = dir
		slog.Info("use default: folder-path", slog.String("folder-path", dir))
	}

	imageName := os.Getenv("IMAGE_NAME")
	if params["image-name"] != "" {
		imageName = params["image-name"]
	}
	if imageName == "" {
		imageName = "test"
		slog.Info("use default: imageName test")
	}

	config := Config{
		Ctx:            ctx,
		Daemon:         buildkitDaemon,
		RegistryUrl:    registryUrl,
		DockerfileName: dockerfileName,
		ImageUrl:       fmt.Sprintf("%s/%s", registryUrl, strings.TrimPrefix(imageName, "/")),
		ImageName:      imageName,
		FolderPath:     folderPath,
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
