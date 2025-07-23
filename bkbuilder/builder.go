package bkbuilder

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/moby/buildkit/util/progress/progresswriter"
	"github.com/tonistiigi/fsutil"
	"golang.org/x/sync/errgroup"
)

func (bkbuilder *BkBuilder) BuildFromDockerfile(ctx context.Context, folderPath string, dockerFilename string, imageUrl string, secrets map[string]string) (*BuildResult, error) {
	ctxFS, err := fsutil.NewFS(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create FS for context %+w", err)
	}

	opt := client.SolveOpt{
		Frontend: "dockerfile.v0",
		LocalMounts: map[string]fsutil.FS{
			"context":    ctxFS,
			"dockerfile": ctxFS,
		},

		FrontendAttrs: map[string]string{
			"filename": dockerFilename,
		},
		Exports: []client.ExportEntry{
			{
				Type: "image",
				Attrs: map[string]string{
					"name":              imageUrl,
					"push":              "true",
					"compression-level": "1",
					"registry.insecure": "true",
				},
			},
		},
	}

	for k, v := range secrets {
		secretMount := secretsprovider.FromMap(map[string][]byte{k: []byte(v)})
		opt.Session = append(opt.Session, secretMount)
	}

	sess, err := session.NewSession(ctx, "build from docker file")
	if err != nil {
		return nil, fmt.Errorf("failed to extablish builkdit clinet session %+w", err)
	}
	if bkbuilder.authProvider != nil {
		sess.Allow(bkbuilder.authProvider)
		opt.Session = append(opt.Session, bkbuilder.authProvider)
	}

	f, err := os.Create("output.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create file %+w", err)
	}

	pWriter, err := progresswriter.NewPrinter(ctx, f, "rawjson") // auto quiet plain rawjson . tty is not allowed here due err: filed to write progress due error provided file is not a console
	if err != nil {
		return nil, fmt.Errorf("failed to write build logs to file %+s due error %+w", f.Name(), err)
	}

	result := &BuildResult{}
	statusCh := pWriter.Status()

	var wg sync.WaitGroup
	wg.Add(2)
	eg, ctx := errgroup.WithContext(ctx)

	go func() {
		defer wg.Done()
		eg.Go(newDisplay(statusCh, "auto"))
		result.updateStatus(statusCh)
	}()
	go func() {
		defer wg.Done()
		solveResponed, err := bkbuilder.Client.Solve(ctx, nil, opt, statusCh)
		result.updateSolveResult(solveResponed, err)
	}()
	wg.Wait()
	return result, nil
}

func newDisplay(statusCh chan *client.SolveStatus, displayMode string) func() error {

	return func() error {
		display, err := progressui.NewDisplay(
			os.Stderr,
			progressui.DisplayMode(displayMode),
			// progressui.WithPhase("BUILDINGGGGG"),
			// progressui.WithDesc("SOMETEXT", "SOMECONSOLE"),
		)
		if err != nil {
			return err
		}

		// UpdateFrom must not use the incoming context.
		// Canceling this context kills the reader of statusCh which blocks buildkit.Client's Solve() indefinitely.
		// Solve() closes statusCh at the end and UpdateFrom returns by reading the closed channel.
		//
		// See https://github.com/superfly/flyctl/pull/2682 for the context.
		_, err = display.UpdateFrom(context.Background(), statusCh)
		return err
	}
}
