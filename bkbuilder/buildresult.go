package bkbuilder

import (
	"errors"
	"fmt"
	"time"

	"github.com/moby/buildkit/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrBuildFailedAdminIssue = errors.New("image build failed due to an infrastructure issue")
var ErrBuildFailedUnknown = errors.New("image build failed due to an Unknown error")
var ErrBuildFailedUserIssue = errors.New("image build failed, review the error, logs your Dockerfile")

type BuildResult struct {
	Error     error
	Done      bool
	Blah      map[string]string
	Logs      []string
	LineError string
}

func (res *BuildResult) Wait() {
	for {
		fmt.Print("waiting\n")
		time.Sleep(time.Second * 1)
		if res.Done {
			return
		}
	}
}

func (res *BuildResult) updateSolveResult(response *client.SolveResponse, err error) {
	res.Done = true
	if err != nil {
		statusConvert := status.Convert(err)

		switch statusConvert.Code() {
		case codes.Unavailable:
			res.Error = fmt.Errorf("%w: %w", ErrBuildFailedUserIssue, err)
		default:
			res.Error = fmt.Errorf("%w: %w", ErrBuildFailedUnknown, err)
		}
	}
	if response != nil {
		res.Blah = response.ExporterResponse
	}
}

func (res *BuildResult) updateStatus(statusCh chan *client.SolveStatus) {
	for status := range statusCh {
		for _, vertex := range status.Vertexes {

			if vertex.Error != "" {
				res.LineError = fmt.Sprintf("------\n> %s:\n  %s\n------", vertex.Name, vertex.Error)
			}
		}
	}
}
