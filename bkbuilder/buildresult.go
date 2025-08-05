package bkbuilder

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/solver/errdefs"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/morikuni/aec"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	grcpstatus "google.golang.org/grpc/status"
)

var ErrBuildFailedAdminIssue = errors.New("image build failed due to an infrastructure issue")
var ErrBuildFailedUnknown = errors.New("image build failed due to an Unknown error")
var ErrBuildFailedUserIssue = errors.New("image build failed, review the error, logs your Dockerfile")
var ErrBuildFailedImageUrlIssue = errors.New("image push failed, review the error")

// BuildResult hold the build result.
type BuildResult struct {
	// Error holds the error that is returned to the user.
	Error error
	// Done notify the user that the build process is complete
	Done bool
	// done notify the build process is finished, it's used internally
	done chan any
	Blah map[string]string
	// Logs holds the Dockerfile build process logs
	// those logs are the same as logs coming from the docker build cli
	Logs []string
	// Warnings collect warnings returned from the build process
	// those warnings are the same as warnings coming from the docker build cli
	Warnings []client.VertexWarning
	// ErrorFromFile hold the Dockerfile name and the line the errors come from
	ErrorFromFile bytes.Buffer
	ImageUrl      string
	ImageDigest   string
}

// updateSolveResult function gets the buildkit client.Solve returned values,
// and update the BuildResult struct accordingly.
//
// The main thing the function does is update the BuildResult.Error with the error issuer.
func (res *BuildResult) updateSolveResult(response *client.SolveResponse, err error) {
	defer func() {
		res.Done = true
		close(res.done)
	}()

	if err != nil {
		// if the sourceError length is greater than 1, this indicates a Dockerfile error and its a user issue.
		sourceError := errdefs.Sources(err)
		if len(sourceError) > 0 {
			res.Error = fmt.Errorf("%w: %w", ErrBuildFailedUserIssue, err)
			for _, s := range sourceError {
				s.Print(&res.ErrorFromFile)
			}
			return
		}

		// Convert the error to get the Grpc error code, https://grpc.io/docs/guides/status-codes/
		statusConvert := grcpstatus.Convert(err)
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

// Collecting logs and warnings from build process, and update the BuildResult struct accordingly
func (res *BuildResult) updateStatus(statusCh chan *client.SolveStatus) {
	logsR, logsW := io.Pipe()
	display, err := progressui.NewDisplay(logsW, progressui.PlainMode)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		statusWarn, _ := display.UpdateFrom(context.Background(), statusCh)
		if len(statusWarn) > 0 {
			res.Warnings = append(res.Warnings, statusWarn...)
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(logsR)
		for scanner.Scan() {
			res.Logs = append(res.Logs, scanner.Text())
		}
	}()
	wg.Wait()
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
		if !ok {
			return
		}
	}
}

// PrintWarnings function print the BuildResult.Warnings same as https://github.com/docker/buildx/blob/1e50e8ddabe108f009b9925e13a321d7c8f99f26/commands/build.go#L740
func (res *BuildResult) PrintWarnings(w io.Writer) {
	warnings := res.Warnings
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintf(w, "\n ")
	sb := &bytes.Buffer{}
	if len(warnings) == 1 {
		fmt.Fprintf(sb, "1 warning found")
	} else {
		fmt.Fprintf(sb, "%d warnings found", len(warnings))
	}

	fmt.Fprintf(sb, ":\n")
	fmt.Fprint(w, aec.Apply(sb.String(), aec.YellowF))

	for _, warn := range warnings {
		fmt.Fprintf(w, " - %s\n", warn.Short)
		if logrus.GetLevel() < logrus.DebugLevel {
			continue
		}
		for _, d := range warn.Detail {
			fmt.Fprintf(w, "%s\n", d)
		}
		if warn.URL != "" {
			fmt.Fprintf(w, "More info: %s\n", warn.URL)
		}
		if warn.SourceInfo != nil && warn.Range != nil {
			src := errdefs.Source{
				Info:   warn.SourceInfo,
				Ranges: warn.Range,
			}
			src.Print(w)
		}
		fmt.Fprintf(w, "\n")
	}
}
