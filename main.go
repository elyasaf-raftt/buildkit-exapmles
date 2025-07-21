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

	go func() {
		for status := range BkBuilder.StatusCh {
			for _, v := range status.Vertexes {
				fmt.Printf("📄 v.Cached=%t v.Name=%s v.Digest=%+v v.Error=%+s v.Inputs=%+v\n", v.Cached, v.Name, v.Digest, v.Error, v.Inputs)
			}
			for _, s := range status.Statuses {
				fmt.Printf("🔄 Status: Vertex=%s | ID=%s | Current=%d | Total=%d | Timestamp=%v\n",
					s.Vertex, s.ID, s.Current, s.Total, s.Timestamp)
			}
		}
	}()

	fmt.Printf("✅ Starting to build image %s\n", argsParsed.ImageUrl)
	err = BkBuilder.BuildFromDockerfile(context.Background(), argsParsed.FolderPath, argsParsed.DockerfileName, argsParsed.ImageUrl)
	if err != nil {
		panic(fmt.Sprintf("❌ Falied to build image due err: %+v", err))
	}
	fmt.Println("✅ Image built and pushed successfully!")
}
