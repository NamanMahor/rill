package rillv1

import (
	"context"
	"fmt"
	"strings"
	"testing"

	runtimev1 "github.com/rilldata/rill/proto/gen/rill/runtime/v1"
	"github.com/rilldata/rill/runtime/drivers"
	"github.com/rilldata/rill/runtime/pkg/activity"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/structpb"

	_ "github.com/rilldata/rill/runtime/drivers/file"
)

func TestRillYAML(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t, map[string]string{
		`rill.yaml`: `
title: Hello world
description: This project says hello to the world

connectors:
- name: my-s3
  type: s3
  defaults:
    region: us-east-1

env:
  foo: bar
`,
	})

	res, err := ParseRillYAML(ctx, repo, "")
	require.NoError(t, err)

	require.Equal(t, res.Title, "Hello world")
	require.Equal(t, res.Description, "This project says hello to the world")

	require.Len(t, res.Connectors, 1)
	require.Equal(t, "my-s3", res.Connectors[0].Name)
	require.Equal(t, "s3", res.Connectors[0].Type)
	require.Len(t, res.Connectors[0].Defaults, 1)
	require.Equal(t, "us-east-1", res.Connectors[0].Defaults["region"])

	require.Len(t, res.Variables, 1)
	require.Equal(t, "foo", res.Variables[0].Name)
	require.Equal(t, "bar", res.Variables[0].Default)
}

func TestComplete(t *testing.T) {
	files := map[string]string{
		// rill.yaml
		`rill.yaml`: ``,
		// init.sql
		`init.sql`: `
{{ configure "version" 2 }}
INSTALL 'hello';
`,
		// source s1
		`sources/s1.yaml`: `
connector: s3
path: hello
`,
		// source s2
		`sources/s2.sql`: `
-- @connector: postgres
-- @refresh.cron: 0 0 * * *
SELECT 1
`,
		// model m1
		`models/m1.sql`: `
SELECT 1
`,
		// model m2
		`models/m2.sql`: `
SELECT * FROM m1
`,
		`models/m2.yaml`: `
materialize: true
`,
		// dashboard d1
		`dashboards/d1.yaml`: `
model: m2
dimensions:
  - name: a
measures:
  - name: b
    expression: count(*)
first_day_of_week: 7
first_month_of_year: 3
`,
		// migration c1
		`custom/c1.yml`: `
kind: migration
version: 3
sql: |
  CREATE TABLE a(a integer);
`,
		// model c2
		`custom/c2.sql`: `
{{ configure "kind" "model" }}
{{ configure "materialize" true }}
SELECT * FROM {{ ref "m2" }}
`,
	}

	truth := true
	resources := []*Resource{
		// init.sql
		{
			Name:  ResourceName{Kind: ResourceKindMigration, Name: "init"},
			Paths: []string{"/init.sql"},
			MigrationSpec: &runtimev1.MigrationSpec{
				Version: 2,
				Sql:     strings.TrimSpace(files["init.sql"]),
			},
		},
		// source s1
		{
			Name:  ResourceName{Kind: ResourceKindSource, Name: "s1"},
			Paths: []string{"/sources/s1.yaml"},
			SourceSpec: &runtimev1.SourceSpec{
				SourceConnector: "s3",
				Properties:      must(structpb.NewStruct(map[string]any{"path": "hello"})),
			},
		},
		// source s2
		{
			Name:  ResourceName{Kind: ResourceKindSource, Name: "s2"},
			Paths: []string{"/sources/s2.sql"},
			SourceSpec: &runtimev1.SourceSpec{
				SourceConnector: "postgres",
				Properties:      must(structpb.NewStruct(map[string]any{"sql": strings.TrimSpace(files["sources/s2.sql"])})),
				RefreshSchedule: &runtimev1.Schedule{Cron: "0 0 * * *"},
			},
		},
		// model m1
		{
			Name:  ResourceName{Kind: ResourceKindModel, Name: "m1"},
			Paths: []string{"/models/m1.sql"},
			ModelSpec: &runtimev1.ModelSpec{
				Sql: strings.TrimSpace(files["models/m1.sql"]),
			},
		},
		// model m2
		{
			Name:  ResourceName{Kind: ResourceKindModel, Name: "m2"},
			Refs:  []ResourceName{{Kind: ResourceKindModel, Name: "m1"}},
			Paths: []string{"/models/m2.yaml", "/models/m2.sql"},
			ModelSpec: &runtimev1.ModelSpec{
				Sql:         strings.TrimSpace(files["models/m2.sql"]),
				Materialize: &truth,
			},
		},
		// dashboard d1
		{
			Name:  ResourceName{Kind: ResourceKindMetricsView, Name: "d1"},
			Refs:  []ResourceName{{Kind: ResourceKindModel, Name: "m2"}},
			Paths: []string{"/dashboards/d1.yaml"},
			MetricsViewSpec: &runtimev1.MetricsViewSpec{
				Table: "m2",
				Dimensions: []*runtimev1.MetricsViewSpec_DimensionV2{
					{Name: "a"},
				},
				Measures: []*runtimev1.MetricsViewSpec_MeasureV2{
					{Name: "b", Expression: "count(*)"},
				},
				FirstDayOfWeek:   7,
				FirstMonthOfYear: 3,
			},
		},
		// migration c1
		{
			Name:  ResourceName{Kind: ResourceKindMigration, Name: "c1"},
			Paths: []string{"/custom/c1.yml"},
			MigrationSpec: &runtimev1.MigrationSpec{
				Version: 3,
				Sql:     "CREATE TABLE a(a integer);",
			},
		},
		// model c2
		{
			Name:  ResourceName{Kind: ResourceKindModel, Name: "c2"},
			Refs:  []ResourceName{{Kind: ResourceKindModel, Name: "m2"}},
			Paths: []string{"/custom/c2.sql"},
			ModelSpec: &runtimev1.ModelSpec{
				Sql:            strings.TrimSpace(files["custom/c2.sql"]),
				Materialize:    &truth,
				UsesTemplating: true,
			},
		},
	}

	ctx := context.Background()
	repo := makeRepo(t, files)
	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, resources, nil)
}

func TestLocationError(t *testing.T) {
	files := map[string]string{
		// rill.yaml
		`rill.yaml`: ``,
		// source s1
		`sources/s1.yaml`: `
connector: s3
path: hello
  world: foo
`,
		// model m1
		`/models/m1.sql`: `
-- @materialize: true
SELECT *

FRO m1
`,
	}

	errors := []*runtimev1.ParseError{
		{
			Message:       " mapping values are not allowed in this context",
			FilePath:      "/sources/s1.yaml",
			StartLocation: &runtimev1.CharLocation{Line: 4},
		},
		{
			Message:       "syntax error at or near",
			FilePath:      "/models/m1.sql",
			StartLocation: &runtimev1.CharLocation{Line: 5},
		},
	}

	ctx := context.Background()
	repo := makeRepo(t, files)
	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, nil, errors)
}

func TestUniqueSourceModelName(t *testing.T) {
	files := map[string]string{
		// rill.yaml
		`rill.yaml`: ``,
		// source s1
		`sources/s1.yaml`: `
connector: s3
`,
		// model s1
		`/models/s1.sql`: `
SELECT 1
`,
	}

	resources := []*Resource{
		{
			Name:  ResourceName{Kind: ResourceKindSource, Name: "s1"},
			Paths: []string{"/sources/s1.yaml"},
			SourceSpec: &runtimev1.SourceSpec{
				SourceConnector: "s3",
				Properties:      must(structpb.NewStruct(map[string]any{})),
			},
		},
	}

	errors := []*runtimev1.ParseError{
		{
			Message:  "model name collides with source \"s1\"",
			FilePath: "/models/s1.sql",
		},
	}

	ctx := context.Background()
	repo := makeRepo(t, files)
	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, resources, errors)
}

func TestReparse(t *testing.T) {
	// Prepare
	truth := true
	ctx := context.Background()

	// Create empty project
	repo := makeRepo(t, map[string]string{`rill.yaml`: ``})
	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, nil, nil)

	// Add a source
	putRepo(t, repo, map[string]string{
		`sources/s1.yaml`: `
connector: s3
path: hello
`,
	})
	s1 := &Resource{
		Name:  ResourceName{Kind: ResourceKindSource, Name: "s1"},
		Paths: []string{"/sources/s1.yaml"},
		SourceSpec: &runtimev1.SourceSpec{
			SourceConnector: "s3",
			Properties:      must(structpb.NewStruct(map[string]any{"path": "hello"})),
		},
	}
	diff, err := p.Reparse(ctx, s1.Paths)
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{s1}, nil)
	require.Equal(t, &Diff{
		Added: []ResourceName{s1.Name},
	}, diff)

	// Add a model
	putRepo(t, repo, map[string]string{
		`models/m1.sql`: `
SELECT * FROM foo
`,
	})
	m1 := &Resource{
		Name:  ResourceName{Kind: ResourceKindModel, Name: "m1"},
		Paths: []string{"/models/m1.sql"},
		ModelSpec: &runtimev1.ModelSpec{
			Sql: "SELECT * FROM foo",
		},
	}
	diff, err = p.Reparse(ctx, m1.Paths)
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{s1, m1}, nil)
	require.Equal(t, &Diff{
		Added: []ResourceName{m1.Name},
	}, diff)

	// Annotate the model with a YAML file
	putRepo(t, repo, map[string]string{
		`models/m1.yaml`: `
materialize: true
`,
	})
	m1.Paths = []string{"/models/m1.sql", "/models/m1.yaml"}
	m1.ModelSpec.Materialize = &truth
	diff, err = p.Reparse(ctx, []string{"/models/m1.yaml"})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{s1, m1}, nil)
	require.Equal(t, &Diff{
		Modified: []ResourceName{m1.Name},
	}, diff)

	// Modify the model's SQL
	putRepo(t, repo, map[string]string{
		`models/m1.sql`: `
SELECT * FROM bar
`,
	})
	m1.ModelSpec.Sql = "SELECT * FROM bar"
	diff, err = p.Reparse(ctx, []string{"/models/m1.sql"})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{s1, m1}, nil)
	require.Equal(t, &Diff{
		Modified: []ResourceName{m1.Name},
	}, diff)

	// Rename the model to collide with the source
	putRepo(t, repo, map[string]string{
		`models/m1.sql`: `
-- @name: s1
SELECT * FROM bar
`,
	})
	diff, err = p.Reparse(ctx, []string{"/models/m1.sql"})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{s1}, []*runtimev1.ParseError{
		{
			Message:  "model name collides with source \"s1\"",
			FilePath: "/models/m1.sql",
		},
		{
			Message:  "model name collides with source \"s1\"",
			FilePath: "/models/m1.yaml",
		},
	})
	require.Equal(t, &Diff{
		Deleted: []ResourceName{m1.Name},
	}, diff)

	// Put m1 back and add a syntax error in the source
	putRepo(t, repo, map[string]string{
		`models/m1.sql`: `
SELECT * FROM bar
`,
		`sources/s1.yaml`: `
connector: s3
path: hello
  world: path
`,
	})
	diff, err = p.Reparse(ctx, []string{"/models/m1.sql", "/sources/s1.yaml"})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{m1}, []*runtimev1.ParseError{{
		Message:       "mapping values are not allowed in this context", // note: approximate string match
		FilePath:      "/sources/s1.yaml",
		StartLocation: &runtimev1.CharLocation{Line: 4},
	}})
	require.Equal(t, &Diff{
		Added:   []ResourceName{m1.Name},
		Deleted: []ResourceName{s1.Name},
	}, diff)

	// Delete the source
	deleteRepo(t, repo, s1.Paths[0])
	diff, err = p.Reparse(ctx, s1.Paths)
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{m1}, nil)
	require.Equal(t, &Diff{}, diff)
}

func TestReparseSourceModelCollision(t *testing.T) {
	// Create project with model m1
	ctx := context.Background()
	repo := makeRepo(t, map[string]string{
		`rill.yaml`: ``,
		`models/m1.sql`: `
SELECT 10
		`,
	})
	m1 := &Resource{
		Name:  ResourceName{Kind: ResourceKindModel, Name: "m1"},
		Paths: []string{"/models/m1.sql"},
		ModelSpec: &runtimev1.ModelSpec{
			Sql: "SELECT 10",
		},
	}
	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{m1}, nil)

	// Add colliding source m1
	putRepo(t, repo, map[string]string{
		`sources/m1.yaml`: `
connector: s3
path: hello
`,
	})
	s1 := &Resource{
		Name:  ResourceName{Kind: ResourceKindSource, Name: "m1"},
		Paths: []string{"/sources/m1.yaml"},
		SourceSpec: &runtimev1.SourceSpec{
			SourceConnector: "s3",
			Properties:      must(structpb.NewStruct(map[string]any{"path": "hello"})),
		},
	}
	diff, err := p.Reparse(ctx, s1.Paths)
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{s1}, []*runtimev1.ParseError{
		{
			Message:  "model name collides with source \"m1\"",
			FilePath: "/models/m1.sql",
		},
	})
	require.Equal(t, &Diff{
		Added:   []ResourceName{s1.Name},
		Deleted: []ResourceName{m1.Name},
	}, diff)

	// Remove colliding source, verify model is restored
	deleteRepo(t, repo, "/sources/m1.yaml")
	diff, err = p.Reparse(ctx, s1.Paths)
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{m1}, nil)
	require.Equal(t, &Diff{
		Added:   []ResourceName{m1.Name},
		Deleted: []ResourceName{s1.Name},
	}, diff)
}

func TestReparseNameCollision(t *testing.T) {
	// Create project with model m1
	ctx := context.Background()
	repo := makeRepo(t, map[string]string{
		`rill.yaml`: ``,
		`models/m1.sql`: `
SELECT 10
		`,
		`models/nested/m1.sql`: `
SELECT 20
		`,
		`models/m2.sql`: `
SELECT * FROM m1
		`,
	})
	m1 := &Resource{
		Name:  ResourceName{Kind: ResourceKindModel, Name: "m1"},
		Paths: []string{"/models/m1.sql"},
		ModelSpec: &runtimev1.ModelSpec{
			Sql: "SELECT 10",
		},
	}
	m1Nested := &Resource{
		Name:  ResourceName{Kind: ResourceKindModel, Name: "m1"},
		Paths: []string{"/models/nested/m1.sql"},
		ModelSpec: &runtimev1.ModelSpec{
			Sql: "SELECT 20",
		},
	}
	m2 := &Resource{
		Name:  ResourceName{Kind: ResourceKindModel, Name: "m2"},
		Paths: []string{"/models/m2.sql"},
		Refs:  []ResourceName{{Kind: ResourceKindModel, Name: "m1"}},
		ModelSpec: &runtimev1.ModelSpec{
			Sql: "SELECT * FROM m1",
		},
	}
	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{m1, m2}, []*runtimev1.ParseError{
		{
			Message:  "name collision",
			FilePath: "/models/nested/m1.sql",
			External: true,
		},
	})

	// Remove colliding model, verify things still work
	deleteRepo(t, repo, "/models/m1.sql")
	diff, err := p.Reparse(ctx, m1.Paths)
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{m1Nested, m2}, nil)
	require.Equal(t, &Diff{
		Modified: []ResourceName{m1.Name, m2.Name}, // m2 due to ref re-inference
	}, diff)
}

func TestReparseMultiKindNameCollision(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t, map[string]string{
		`rill.yaml`:            ``,
		`models/m1.sql`:        `SELECT 10`,
		`models/nested/m1.sql`: `SELECT 20`,
		`sources/m1.yaml`: `
type: s3
path: hello
`,
	})
	src := &Resource{
		Name:  ResourceName{Kind: ResourceKindSource, Name: "m1"},
		Paths: []string{"/sources/m1.yaml"},
		SourceSpec: &runtimev1.SourceSpec{
			SourceConnector: "s3",
			Properties:      must(structpb.NewStruct(map[string]any{"path": "hello"})),
		},
	}
	mdl := &Resource{
		Name:  ResourceName{Kind: ResourceKindModel, Name: "m1"},
		Paths: []string{"/models/m1.sql"},
		ModelSpec: &runtimev1.ModelSpec{
			Sql: "SELECT 10",
		},
	}

	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{src}, []*runtimev1.ParseError{
		{
			Message:  "collides with source",
			FilePath: "/models/m1.sql",
			External: true,
		},
		{
			Message:  "name collision",
			FilePath: "/models/nested/m1.sql",
			External: true,
		},
	})

	// Delete source m1
	deleteRepo(t, repo, "/sources/m1.yaml")
	diff, err := p.Reparse(ctx, src.Paths)
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{mdl}, []*runtimev1.ParseError{
		{
			Message:  "name collision",
			FilePath: "/models/nested/m1.sql",
			External: true,
		},
	})
	require.Equal(t, &Diff{
		Added:   []ResourceName{mdl.Name},
		Deleted: []ResourceName{src.Name},
	}, diff)
}

func TestReparseRillYAML(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t, map[string]string{})

	mdl := &Resource{
		Name:  ResourceName{Kind: ResourceKindModel, Name: "m1"},
		Paths: []string{"/models/m1.sql"},
		ModelSpec: &runtimev1.ModelSpec{
			Sql: "SELECT 10",
		},
	}
	perr := &runtimev1.ParseError{
		Message:  "rill.yaml not found",
		FilePath: "/rill.yaml",
	}

	// Parse empty project. Expect rill.yaml error.
	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	require.Nil(t, p.RillYAML)
	requireResourcesAndErrors(t, p, nil, []*runtimev1.ParseError{perr})

	// Add rill.yaml. Expect success.
	putRepo(t, repo, map[string]string{
		`rill.yaml`: ``,
	})
	diff, err := p.Reparse(ctx, []string{"/rill.yaml"})
	require.NoError(t, err)
	require.True(t, diff.Reloaded)
	require.NotNil(t, p.RillYAML)
	requireResourcesAndErrors(t, p, nil, nil)

	// Remove rill.yaml and add a model. Expect reloaded.
	deleteRepo(t, repo, "/rill.yaml")
	putRepo(t, repo, map[string]string{"/models/m1.sql": "SELECT 10"})
	diff, err = p.Reparse(ctx, []string{"/rill.yaml", "/models/m1.sql"})
	require.NoError(t, err)
	require.True(t, diff.Reloaded)
	require.Nil(t, p.RillYAML)
	requireResourcesAndErrors(t, p, []*Resource{mdl}, []*runtimev1.ParseError{perr})

	// Edit model. Expect nothing to happen because rill.yaml is still broken.
	putRepo(t, repo, map[string]string{"/models/m1.sql": "SELECT 20"})
	diff, err = p.Reparse(ctx, []string{"/models/m1.sql"})
	require.NoError(t, err)
	require.Equal(t, &Diff{Skipped: true}, diff)
	require.Nil(t, p.RillYAML)
	requireResourcesAndErrors(t, p, []*Resource{mdl}, []*runtimev1.ParseError{perr})

	// Fix rill.yaml. Expect reloaded.
	mdl.ModelSpec.Sql = "SELECT 20"
	putRepo(t, repo, map[string]string{"/rill.yaml": ""})
	diff, err = p.Reparse(ctx, []string{"/rill.yaml"})
	require.NoError(t, err)
	require.True(t, diff.Reloaded)
	require.NotNil(t, p.RillYAML)
	requireResourcesAndErrors(t, p, []*Resource{mdl}, nil)
}

func TestRefInferrence(t *testing.T) {
	// Create model referencing "bar"
	foo := &Resource{
		Name:  ResourceName{Kind: ResourceKindModel, Name: "foo"},
		Paths: []string{"/models/foo.sql"},
		ModelSpec: &runtimev1.ModelSpec{
			Sql: "SELECT * FROM bar",
		},
	}
	ctx := context.Background()
	repo := makeRepo(t, map[string]string{
		// rill.yaml
		`rill.yaml`: ``,
		// model foo
		`models/foo.sql`: `SELECT * FROM bar`,
	})
	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{foo}, nil)

	// Add model "bar"
	foo.Refs = []ResourceName{{Kind: ResourceKindModel, Name: "bar"}}
	bar := &Resource{
		Name:  ResourceName{Kind: ResourceKindModel, Name: "bar"},
		Paths: []string{"/models/bar.sql"},
		ModelSpec: &runtimev1.ModelSpec{
			Sql: "SELECT * FROM baz",
		},
	}
	putRepo(t, repo, map[string]string{
		`models/bar.sql`: `SELECT * FROM baz`,
	})
	diff, err := p.Reparse(ctx, []string{"/models/bar.sql"})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{foo, bar}, nil)
	require.Equal(t, &Diff{
		Added:    []ResourceName{bar.Name},
		Modified: []ResourceName{foo.Name},
	}, diff)

	// Remove "bar"
	foo.Refs = nil
	deleteRepo(t, repo, bar.Paths[0])
	diff, err = p.Reparse(ctx, []string{"/models/bar.sql"})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, []*Resource{foo}, nil)
	require.Equal(t, &Diff{
		Modified: []ResourceName{foo.Name},
		Deleted:  []ResourceName{bar.Name},
	}, diff)
}

func BenchmarkReparse(b *testing.B) {
	ctx := context.Background()
	truth := true
	files := map[string]string{
		// rill.yaml
		`rill.yaml`: ``,
		// model m1
		`models/m1.sql`: `
SELECT 1
`,
		// model m2
		`models/m2.sql`: `
SELECT * FROM m1
`,
		`models/m2.yaml`: `
materialize: true
`,
	}
	resources := []*Resource{
		// m1
		{
			Name:  ResourceName{Kind: ResourceKindModel, Name: "m1"},
			Paths: []string{"/models/m1.sql"},
			ModelSpec: &runtimev1.ModelSpec{
				Sql: strings.TrimSpace(files["models/m1.sql"]),
			},
		},
		// m2
		{
			Name:  ResourceName{Kind: ResourceKindModel, Name: "m2"},
			Refs:  []ResourceName{{Kind: ResourceKindModel, Name: "m1"}},
			Paths: []string{"/models/m2.sql", "/models/m2.yaml"},
			ModelSpec: &runtimev1.ModelSpec{
				Sql:         strings.TrimSpace(files["models/m2.sql"]),
				Materialize: &truth,
			},
		},
	}
	repo := makeRepo(b, files)
	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(b, err)
	requireResourcesAndErrors(b, p, resources, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		files[`models/m2.sql`] = fmt.Sprintf(`SELECT * FROM m1 LIMIT %d`, i)
		_, err = p.Reparse(ctx, []string{`models/m2.sql`})
		require.NoError(b, err)
		require.Empty(b, p.Errors)
	}
}

func TestProjectModelDefaults(t *testing.T) {
	ctx := context.Background()
	truth := true
	falsity := false

	files := map[string]string{
		// Provide dashboard defaults in rill.yaml
		`rill.yaml`: `
models:
  materialize: true
`,
		// Model that inherits defaults
		`models/m1.sql`: `
SELECT * FROM t1
`,
		// Model that overrides defaults
		`models/m2.sql`: `
-- @materialize: false
SELECT * FROM t2
`,
	}

	resources := []*Resource{
		// m1
		{
			Name:  ResourceName{Kind: ResourceKindModel, Name: "m1"},
			Paths: []string{"/models/m1.sql"},
			ModelSpec: &runtimev1.ModelSpec{
				Sql:         strings.TrimSpace(files["models/m1.sql"]),
				Materialize: &truth,
			},
		},
		// m2
		{
			Name:  ResourceName{Kind: ResourceKindModel, Name: "m2"},
			Paths: []string{"/models/m2.sql"},
			ModelSpec: &runtimev1.ModelSpec{
				Sql:         strings.TrimSpace(files["models/m2.sql"]),
				Materialize: &falsity,
			},
		},
	}

	repo := makeRepo(t, files)
	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, resources, nil)
}

func TestProjectDashboardDefaults(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t, map[string]string{
		// Provide dashboard defaults in rill.yaml
		`rill.yaml`: `
dashboards:
  first_day_of_week: 7
  available_time_zones:
    - America/New_York
  security:
    access: true
`,
		// Dashboard that inherits defaults
		`dashboards/d1.yaml`: `
table: t1
dimensions:
  - name: a
measures:
  - name: b
    expression: count(*)
`,
		// Dashboard that overrides defaults
		`dashboards/d2.yaml`: `
table: t2
dimensions:
  - name: a
measures:
  - name: b
    expression: count(*)
first_day_of_week: 1
available_time_zones: []
security:
  row_filter: true
`,
	})

	resources := []*Resource{
		// dashboard d1
		{
			Name:  ResourceName{Kind: ResourceKindMetricsView, Name: "d1"},
			Paths: []string{"/dashboards/d1.yaml"},
			MetricsViewSpec: &runtimev1.MetricsViewSpec{
				Table: "t1",
				Dimensions: []*runtimev1.MetricsViewSpec_DimensionV2{
					{Name: "a"},
				},
				Measures: []*runtimev1.MetricsViewSpec_MeasureV2{
					{Name: "b", Expression: "count(*)"},
				},
				FirstDayOfWeek:     7,
				AvailableTimeZones: []string{"America/New_York"},
				Security: &runtimev1.MetricsViewSpec_SecurityV2{
					Access: "true",
				},
			},
		},
		// dashboard d2
		{
			Name:  ResourceName{Kind: ResourceKindMetricsView, Name: "d2"},
			Paths: []string{"/dashboards/d2.yaml"},
			MetricsViewSpec: &runtimev1.MetricsViewSpec{
				Table: "t2",
				Dimensions: []*runtimev1.MetricsViewSpec_DimensionV2{
					{Name: "a"},
				},
				Measures: []*runtimev1.MetricsViewSpec_MeasureV2{
					{Name: "b", Expression: "count(*)"},
				},
				FirstDayOfWeek:     1,
				AvailableTimeZones: []string{},
				Security: &runtimev1.MetricsViewSpec_SecurityV2{
					Access:    "true",
					RowFilter: "true",
				},
			},
		},
	}

	p, err := Parse(ctx, repo, "", "", []string{""})
	require.NoError(t, err)
	requireResourcesAndErrors(t, p, resources, nil)
}

func requireResourcesAndErrors(t testing.TB, p *Parser, wantResources []*Resource, wantErrors []*runtimev1.ParseError) {
	// Check resources
	gotResources := maps.Clone(p.Resources)
	for _, want := range wantResources {
		found := false
		for _, got := range gotResources {
			if want.Name == got.Name {
				require.Equal(t, want.Name, got.Name)
				require.ElementsMatch(t, want.Refs, got.Refs, "for resource %q", want.Name)
				require.ElementsMatch(t, want.Paths, got.Paths, "for resource %q", want.Name)
				require.Equal(t, want.SourceSpec, got.SourceSpec, "for resource %q", want.Name)
				require.Equal(t, want.ModelSpec, got.ModelSpec, "for resource %q", want.Name)
				require.Equal(t, want.MetricsViewSpec, got.MetricsViewSpec, "for resource %q", want.Name)
				require.Equal(t, want.MigrationSpec, got.MigrationSpec, "for resource %q", want.Name)

				delete(gotResources, got.Name)
				found = true
				break
			}
		}
		require.True(t, found, "missing resource %q", want.Name)
	}
	require.True(t, len(gotResources) == 0, "unexpected resources: %v", gotResources)

	// Check errors
	// NOTE: Assumes there's at most one parse error per file path
	// NOTE: Matches error messages using Contains (exact match not required)
	gotErrors := slices.Clone(p.Errors)
	for _, want := range wantErrors {
		found := false
		for i, got := range gotErrors {
			if want.FilePath == got.FilePath {
				require.Contains(t, got.Message, want.Message, "for path %q", got.FilePath)
				require.Equal(t, want.StartLocation, got.StartLocation, "for path %q", got.FilePath)
				gotErrors = slices.Delete(gotErrors, i, i+1)
				found = true
				break
			}
		}
		require.True(t, found, "missing error for path %q", want.FilePath)
	}
	require.True(t, len(gotErrors) == 0, "unexpected errors: %v", gotErrors)
}

func makeRepo(t testing.TB, files map[string]string) drivers.RepoStore {
	root := t.TempDir()
	handle, err := drivers.Open("file", map[string]any{"dsn": root}, false, activity.NewNoopClient(), zap.NewNop())
	require.NoError(t, err)

	repo, ok := handle.AsRepoStore("")
	require.True(t, ok)

	putRepo(t, repo, files)

	return repo
}

func putRepo(t testing.TB, repo drivers.RepoStore, files map[string]string) {
	for path, data := range files {
		err := repo.Put(context.Background(), path, strings.NewReader(data))
		require.NoError(t, err)
	}
}

func deleteRepo(t testing.TB, repo drivers.RepoStore, files ...string) {
	for _, path := range files {
		err := repo.Delete(context.Background(), path)
		require.NoError(t, err)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
