package bkbuilder

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/solver/errdefs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrBuildFailedAdminIssue = errors.New("image build failed due to an infrastructure issue")
var ErrBuildFailedUnknown = errors.New("image build failed due to an Unknown error")
var ErrBuildFailedUserIssue = errors.New("image build failed, review the error, logs your Dockerfile")

type BuildResult struct {
	Error         error
	Done          bool
	Blah          map[string]string
	Logs          []string
	ErrorFromFile bytes.Buffer
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
	defer func() { res.Done = true }()
	if err != nil {

		for _, s := range errdefs.Sources(err) {
			res.Error = fmt.Errorf("%w: %w", ErrBuildFailedUserIssue, err)
			s.Print(&res.ErrorFromFile)
			return
		}
		if strings.Contains(err.Error(), "no such file or directory") {
			res.Error = fmt.Errorf("%w: %w", ErrBuildFailedUserIssue, err)
			return
		}
		if strings.Contains(err.Error(), "the Dockerfile cannot be empty") {
			res.Error = fmt.Errorf("%w: %w", ErrBuildFailedUserIssue, err)
			return
		}

		statusConvert := status.Convert(err)
		switch statusConvert.Code() {
		case codes.Unavailable:
			res.Error = fmt.Errorf("%w: %w", ErrBuildFailedAdminIssue, err)
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
		for _, v := range status.Vertexes {
			res.Logs = append(res.Logs, v.Name)
		}
	}
}
