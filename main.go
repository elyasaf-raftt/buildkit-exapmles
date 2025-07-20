package main

import (
	"fmt"
	"log/slog"

	"github.com/moby/buildkit/client"

	"invbuildkit/buildkitconfig"

	"golang.org/x/sync/errgroup"
)

func main() {
	buildkitConfig, err := buildkitconfig.GetConfig()

	if err != nil {
		slog.Error("Failed to config buildkit daemon due", slog.String("error", err.Error()))
	}

	ctx := buildkitConfig.Ctx

	bkClient, err := client.New(ctx, buildkitConfig.Daemon)
	if err != nil {
		slog.Error("failed to connect to buildkit", slog.String("error", err.Error()))
	}
	defer bkClient.Close()

	opt, err := buildkitconfig.GetSolveOptForDockerfile(buildkitConfig)
	if err != nil {
		slog.Error("failed to get opt for dockerfile", slog.String("error", err.Error()))
	}

	statusCh := make(chan *client.SolveStatus)
	eg, ctx := errgroup.WithContext(ctx)
	go func() {
		for status := range statusCh {
			for _, v := range status.Vertexes {
				fmt.Printf("📄 v.Cached=%t v.Name=%s v.Digest=%+v v.Error=%+s v.Inputs=%+v\n", v.Cached, v.Name, v.Digest, v.Error, v.Inputs)
			}
			for _, s := range status.Statuses {
				fmt.Printf("🔄 Status: Vertex=%s | ID=%s | Current=%d | Total=%d | Timestamp=%v\n",
					s.Vertex, s.ID, s.Current, s.Total, s.Timestamp)
			}
		}
	}()
	var res *client.SolveResponse
	eg.Go(func() error {
		slog.Info("starting to build image", slog.String("image name", buildkitConfig.Image))
		res, err = bkClient.Solve(ctx, nil, opt, statusCh)
		if err != nil {
			slog.Error("failed to buildkimage", slog.String("error", err.Error()))
		}
		slog.Debug("", slog.Any("respond", res.ExporterResponse))
		return nil
	})
	err = eg.Wait()
	slog.Debug("", slog.Any("image digest", res.ExporterResponse))

	fmt.Println("✅ Image built and pushed successfully!")
}
