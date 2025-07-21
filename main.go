package main

import (
	"context"
	"fmt"
	"log/slog"

	"invbuildkit/bkbuilder"
	"invbuildkit/buildkitconfig"

	"github.com/moby/buildkit/client"
)

func main() {

	argsParsed, err := buildkitconfig.ParseArgs()

	if err != nil {
		slog.Error("Failed to config buildkit daemon due", slog.String("error", err.Error()))
	}

	ctx := argsParsed.Ctx
	BkBuilder := bkbuilder.NewBkBuilder(ctx, argsParsed.Daemon)

	bkClient, err := client.New(ctx, argsParsed.Daemon)
	if err != nil {
		slog.Error("failed to connect to buildkit", slog.String("error", err.Error()))
	}
	defer bkClient.Close()

	fmt.Printf("✅ Starting to build image %s\n", argsParsed.ImageUrl)
	err = BkBuilder.BuildFromDockerfile(context.Background(), bkClient, argsParsed.FolderPath, argsParsed.DockerfileName, argsParsed.ImageUrl)
	if err != nil {
		panic(fmt.Sprintf("❌ Falied to build image due err: %+v", err))
	}
	fmt.Println("✅ Image built and pushed successfully!")
}
