package bkbuilder

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/tonistiigi/fsutil"
)

func (bkbuilder *BkBuilder) BuildFromDockerfile(ctx context.Context, folderPath string, dockerFilename string, imageUrl string) error {
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

	res, err := bkbuilder.Client.Solve(ctx, nil, opt, bkbuilder.StatusCh)
	if err != nil {
		return fmt.Errorf("failed to build image %+w", err)
	}
	slog.Debug("", slog.Any("respond", res.ExporterResponse))
	fmt.Printf("%+v", res.ExporterResponse)
	return nil
}
