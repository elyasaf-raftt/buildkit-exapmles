package main

import (
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

	fmt.Println("sending build")
	buildResult, err := builder.BuildFromDockerfile(ctx, argsParsed.FolderPath, "Dockerfile-test", argsParsed.ImageUrl)
	if err != nil {
		handleErr(err)
	}
	fmt.Println("waiting for build")

	updateStatus := make(chan string, 1)
	go func() {
		for status := range updateStatus {
			if status != "" {
				fmt.Printf("%s\n", status)
			}
		}
	}()
	buildResult.WaitForStatus(updateStatus)
	// buildResult.Wait()
	close(updateStatus)

	if buildResult.Error != nil {
		fmt.Println("*********** Build Failed **************")
		fmt.Println(buildResult.ErrorFromFile.String())
		handleErr(buildResult.Error)
	}

	for _, l := range buildResult.Logs {
		// Do Somthing with logs
		_ = l
		// fmt.Println(l)
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
