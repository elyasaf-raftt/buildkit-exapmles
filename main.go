package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"invbuildkit/bkbuilder"
	"invbuildkit/buildkitconfig"
	"log/slog"
	"os"
)

func main() {
	ctx := context.Background()

	argsParsed, err := buildkitconfig.ParseArgs()
	if err != nil {
		slog.Error("Failed to config buildkit daemon due", slog.String("error", err.Error()))
	}

	fmt.Println("initializing buildkit")

	builder, err := bkbuilder.New(ctx)
	if err != nil {
		handleErr(err)
	}
	defer builder.Close()

	var buildArg bkbuilder.BuildArgs = map[string]string{}
	var noCache bkbuilder.NoCache = false

	buildResult, err := builder.BuildFromDockerfile(ctx, argsParsed.FolderPath, "Dockerfile-test", argsParsed.ImageUrl, buildArg, noCache)
	if err != nil {
		handleErr(err)
	}
	fmt.Println("waiting for build")
	buildResult.Wait()

	fmt.Println("sending build")
	for _, l := range buildResult.Logs {
		fmt.Println(l)
	}
	var buf bytes.Buffer
	buildResult.PrintWarnings(&buf)
	fmt.Println(buf.String())

	if buildResult.Error != nil {
		fmt.Println("*********** Build Failed **************")
		fmt.Println(buildResult.ErrorFromFile.String())
		handleErr(buildResult.Error)
	}

	fmt.Printf("Successful to build and push image\nUrl: %s\nDigest: %s\n", buildResult.ImageUrl, buildResult.ImageDigest)
}

func handleErr(err error) {
	origErr := err
	for {
		nextErr := errors.Unwrap(origErr)
		if nextErr != nil {
			origErr = nextErr
		} else {
			break
		}
	}
	slog.Error(err.Error(), slog.String("ErrorType", fmt.Sprintf("%T", origErr)))
	os.Exit(1)
}
