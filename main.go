package main

import (
	"context"
	"fmt"
	"log/slog"

	"invbuildkit/bkbuilder"
	"invbuildkit/buildkitconfig"
)

func main() {

	argsParsed, err := buildkitconfig.ParseArgs()

	if err != nil {
		slog.Error("Failed to config buildkit daemon due", slog.String("error", err.Error()))
	}

	ctx := argsParsed.Ctx
	BkBuilder := bkbuilder.NewBkBuilder(ctx, argsParsed.Daemon)
	defer BkBuilder.Close()

	fmt.Printf("✅ Starting to build image %s\n", argsParsed.ImageUrl)
	err = BkBuilder.BuildFromDockerfile(context.Background(), argsParsed.FolderPath, argsParsed.DockerfileName, argsParsed.ImageUrl)
	if err != nil {
		panic(fmt.Sprintf("❌ Falied to build image due err: %+v", err))
	}
	fmt.Println("✅ Image built and pushed successfully!")
}
