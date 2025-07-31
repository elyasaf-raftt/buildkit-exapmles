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
var ErrBuildFailedImageUrlIssue = errors.New("image push failed, review the error")

// BuildResult hold the build result.
type BuildResult struct {
	// Error holds the error that is returned to the user.
	Error error
	// Done indicate that data writing is finished.
	Done bool
	done chan any
	Blah map[string]string
	// Logs holds the Dockerfile build process logs
	// those logs are the same as logs coming from the docker build cli
	Logs []string
	// ErrorFromFile hold the Dockerfile name and the line the error come from
	ErrorFromFile bytes.Buffer
	ImageUrl      string
	ImageDigest   string
}

// Function updateSolveResult get the buildkit client.Solve return values,
// and looks at those values to indicate the issuer error
func (res *BuildResult) updateSolveResult(response *client.SolveResponse, err error) {
	defer func() {
		res.Done = true
		close(res.done)
	}()

	if err != nil {
		// if the sourceError length is greater than 1, this indicates a Dockerfile error and its a user issue
		sourceError := errdefs.Sources(err)
		if len(sourceError) > 0 {
			res.Error = fmt.Errorf("%w: %w", ErrBuildFailedUserIssue, err)
			for _, s := range sourceError {
				s.Print(&res.ErrorFromFile)
			}
			return
		}

		// Convert the error to get the Grpc error code, https://grpc.io/docs/guides/status-codes/
		statusConvert := status.Convert(err)
		switch statusConvert.Code() {
		// Connection failed its a admin issue
		case codes.Unavailable:
			res.Error = fmt.Errorf("%w: %w", ErrBuildFailedAdminIssue, err)
		case codes.Unknown:
			// even if the Grpc error it's Unknown sometime we can find the issuer error
			if isUserIssue(err) {
				res.Error = fmt.Errorf("%w: %w", ErrBuildFailedUserIssue, err)
			} else if isAdminIssue(err) {
				res.Error = fmt.Errorf("%w: %w", ErrBuildFailedAdminIssue, err)
			} else if isImageUrlIssue(err) {
				res.Error = fmt.Errorf("%w: %w", ErrBuildFailedImageUrlIssue, err)
			} else {
				res.Error = fmt.Errorf("%w: %w", ErrBuildFailedUnknown, err)
			}
		default:
			res.Error = fmt.Errorf("%w: %w", ErrBuildFailedUnknown, err)
		}
	}

	if response != nil {
		res.Blah = response.ExporterResponse
		res.ImageDigest = response.ExporterResponse["containerimage.digest"]
	}
}

// Collecting logs to be used by who called this package
func (res *BuildResult) updateStatus(statusCh chan *client.SolveStatus) {
	for status := range statusCh {
		for _, v := range status.Vertexes {
			if len(status.Vertexes) > 1 {
				continue
			}
			res.Logs = append(res.Logs, v.Name)
			fmt.Printf("%+v\n", v.Name)
		}
		// jsonStatus, _ := json.Marshal(status)
		// fmt.Printf("status=%+v\n", string(jsonStatus))
	}
}

// Look for specific cases to see if it's a user issue
func isUserIssue(err error) bool {
	noSuchError := strings.Contains(err.Error(), "no such file or directory")
	emptyError := strings.Contains(err.Error(), "the Dockerfile cannot be empty")

	return noSuchError || emptyError
}

// Look for specific cases to see if it's a admin issue
func isAdminIssue(err error) bool {
	// connection refused
	failedToPush := strings.Contains(err.Error(), "failed to do request")
	// repository does not exist or may require authorization
	accessDenied := strings.Contains(err.Error(), "push access denied")
	unauthorized := strings.Contains(err.Error(), "401 Unauthorized")

	return failedToPush || accessDenied || unauthorized
}

// Look for specific cases to see if it's a admin issue
func isImageUrlIssue(err error) bool {
	// invalid image name
	imageUrlError := strings.Contains(err.Error(), "invalid reference format")
	return imageUrlError
}

// Wait until the build process done
func (res *BuildResult) Wait() {
	for {
		fmt.Print("waiting\n")
		time.Sleep(time.Second * 1)
		_, ok := <-res.done
		fmt.Printf("ok=%+v", ok)
		if !ok {
			return
		}
	}
}
