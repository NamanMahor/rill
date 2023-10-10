package server

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"

	runtimev1 "github.com/rilldata/rill/proto/gen/rill/runtime/v1"
	"github.com/rilldata/rill/runtime/compilers/rillv1"
	"github.com/rilldata/rill/runtime/pkg/examples"
	"github.com/rilldata/rill/runtime/pkg/observability"
	"github.com/rilldata/rill/runtime/server/auth"
	"go.opentelemetry.io/otel/attribute"
)

// ListExamples returns a list of embedded examples
func (s *Server) ListExamples(ctx context.Context, req *runtimev1.ListExamplesRequest) (*runtimev1.ListExamplesResponse, error) {
	list, err := examples.List()
	if err != nil {
		return nil, err
	}

	resp := make([]*runtimev1.Example, len(list))
	for i, example := range list {
		resp[i] = &runtimev1.Example{
			Name:        example.Name,
			Title:       example.Title,
			Description: example.Description,
		}
	}

	return &runtimev1.ListExamplesResponse{
		Examples: resp,
	}, nil
}

func (s *Server) UnpackExample(ctx context.Context, req *runtimev1.UnpackExampleRequest) (*runtimev1.UnpackExampleResponse, error) {
	observability.AddRequestAttributes(ctx,
		attribute.String("args.instance_id", req.InstanceId),
		attribute.String("args.name", req.Name),
		attribute.Bool("args.force", req.Force),
	)

	s.addInstanceRequestAttributes(ctx, req.InstanceId)

	if !auth.GetClaims(ctx).CanInstance(req.InstanceId, auth.EditRepo) {
		return nil, ErrForbidden
	}

	repo, release, err := s.runtime.Repo(ctx, req.InstanceId)
	if err != nil {
		return nil, err
	}
	defer release()

	exampleFS, err := examples.Get(req.Name)
	if err != nil {
		return nil, err
	}

	existingPaths := make(map[string]bool)
	if !req.Force {
		paths, err := repo.ListRecursive(ctx, "**")
		if err != nil {
			return nil, err
		}

		for _, path := range paths {
			existingPaths[path] = true
		}
	}

	paths := make([]string, 0)
	err = fs.WalkDir(exampleFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if _, ok := existingPaths[filepath.Join("/", path)]; ok {
			return fmt.Errorf("path %q already exists", path)
		}

		paths = append(paths, path)
		return nil
	})

	if err != nil {
		return nil, err
	}

	for _, path := range paths {
		err = func() error {
			file, err := exampleFS.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			return repo.Put(ctx, path, file)
		}()
		if err != nil {
			return nil, err
		}
	}

	return &runtimev1.UnpackExampleResponse{}, nil
}

func (s *Server) UnpackEmpty(ctx context.Context, req *runtimev1.UnpackEmptyRequest) (*runtimev1.UnpackEmptyResponse, error) {
	observability.AddRequestAttributes(ctx,
		attribute.String("args.instance_id", req.InstanceId),
		attribute.String("args.title", req.Title),
		attribute.Bool("args.force", req.Force),
	)

	s.addInstanceRequestAttributes(ctx, req.InstanceId)

	if !auth.GetClaims(ctx).CanInstance(req.InstanceId, auth.EditRepo) {
		return nil, ErrForbidden
	}

	repo, release, err := s.runtime.Repo(ctx, req.InstanceId)
	if err != nil {
		return nil, err
	}
	defer release()

	if rillv1.IsInit(ctx, repo, req.InstanceId) && !req.Force {
		return nil, fmt.Errorf("a Rill project already exists")
	}

	// Init empty project
	err = rillv1.InitEmpty(ctx, repo, req.InstanceId, req.Title)
	if err != nil {
		return nil, err
	}

	return &runtimev1.UnpackEmptyResponse{}, nil
}
