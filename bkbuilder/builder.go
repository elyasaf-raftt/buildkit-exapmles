package bkbuilder

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/tonistiigi/fsutil"
	"golang.org/x/sync/errgroup"
)

func (bkbuilder *BkBuilder) BuildFromDockerfile(ctx context.Context, bkclient *client.Client, folderPath string, dockerFilename string, imageUrl string) error {
	ctxFS, err := fsutil.NewFS(folderPath)
	if err != nil {
		return fmt.Errorf("failed to create FS for context %+w", err)
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

	sess, err := session.NewSession(ctx, "build from docker file")
	if err != nil {
		return fmt.Errorf("failed to extablish builkdit clinet session %+w", err)
	}
	if bkbuilder.authProvider != nil {
		sess.Allow(bkbuilder.authProvider)
		opt.Session = append(opt.Session, bkbuilder.authProvider)
	}

	statusCh := make(chan *client.SolveStatus)

	eg, ctx := errgroup.WithContext(ctx)
	go func() {
		fmt.Printf("%+v", statusCh)
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
		res, err = bkclient.Solve(ctx, nil, opt, statusCh)
		if err != nil {
			return fmt.Errorf("failed to build image %+w", err)
		}
		slog.Debug("", slog.Any("respond", res.ExporterResponse))
		fmt.Printf("%+v", res.ExporterResponse)
		return nil
	})
	err = eg.Wait()
	if err != nil {
		return err
	}

	return nil
}
