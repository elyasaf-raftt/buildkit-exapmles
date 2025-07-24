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
var ErrBuildFailedUnknown = errors.New("image build failed due to an anknown error")
var ErrBuildFailedUserIssue = errors.New("image build failed, review the error, logs your Dockerfile")

type BuildResult struct {
	Error error
	Done  bool
	Blah  map[string]string
	Logs  []string
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
	// Do Somthing

	// for status := range statusCh {
	// 	for _, v := range status.Vertexes {
	// 		fmt.Printf("📄 v.Cached=%t v.Name=%s v.Digest=%+v v.Error=%+s v.Inputs=%+v\n", v.Cached, v.Name, v.Digest, v.Error, v.Inputs)
	// 	}
	// 	for _, s := range status.Statuses {
	// 		fmt.Printf("🔄 Status: Vertex=%s | ID=%s | Current=%d | Total=%d | Timestamp=%v\n",
	// 			s.Vertex, s.ID, s.Current, s.Total, s.Timestamp)
	// 	}
	// }

}
