package runtime_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	runtimev1 "github.com/rilldata/rill/proto/gen/rill/runtime/v1"
	"github.com/rilldata/rill/runtime"
	"github.com/rilldata/rill/runtime/drivers"
	"github.com/rilldata/rill/runtime/testruntime"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestComplete(t *testing.T) {
	// Add source, model, and dashboard
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
		"/models/bar.sql": `
SELECT * FROM foo
`,
		"/metrics/foobar.yaml": `
version: 1
type: metrics_view
model: bar
dimensions:
- name: a
  column: a
measures:
- name: b
  expression: count(*)
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)

	// Verify the source
	testruntime.RequireResource(t, rt, id, &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindSource, Name: "foo"},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{"/sources/foo.yaml"},
		},
		Resource: &runtimev1.Resource_Source{
			Source: &runtimev1.SourceV2{
				Spec: &runtimev1.SourceSpec{
					SourceConnector: "local_file",
					SinkConnector:   "duckdb",
					Properties:      must(structpb.NewStruct(map[string]any{"path": "data/foo.csv"})),
					RefreshSchedule: &runtimev1.Schedule{RefUpdate: true},
				},
				State: &runtimev1.SourceState{
					Connector: "duckdb",
					Table:     "foo",
				},
			},
		},
	})
	testruntime.RequireOLAPTable(t, rt, id, "foo")
	testruntime.RequireOLAPTableCount(t, rt, id, "foo", 3)

	// Verify the model
	testruntime.RequireResource(t, rt, id, &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindModel, Name: "bar"},
			Refs:      []*runtimev1.ResourceName{{Kind: runtime.ResourceKindSource, Name: "foo"}},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{"/models/bar.sql"},
		},
		Resource: &runtimev1.Resource_Model{
			Model: &runtimev1.ModelV2{
				Spec: &runtimev1.ModelSpec{
					RefreshSchedule: &runtimev1.Schedule{RefUpdate: true},
					InputConnector:  "duckdb",
					InputProperties: must(structpb.NewStruct(map[string]any{"sql": "SELECT * FROM foo"})),
					OutputConnector: "duckdb",
				},
				State: &runtimev1.ModelState{
					ExecutorConnector: "duckdb",
					ResultConnector:   "duckdb",
					ResultProperties:  must(structpb.NewStruct(map[string]any{"table": "bar", "used_model_name": true, "view": true})),
					ResultTable:       "bar",
				},
			},
		},
	})
	testruntime.RequireOLAPTable(t, rt, id, "bar")
	testruntime.RequireOLAPTableCount(t, rt, id, "bar", 3)

	// Verify the metrics view
	mvSpec := &runtimev1.MetricsViewSpec{
		Connector:  "duckdb",
		Model:      "bar",
		Dimensions: []*runtimev1.MetricsViewSpec_DimensionV2{{Name: "a", Column: "a"}},
		Measures:   []*runtimev1.MetricsViewSpec_MeasureV2{{Name: "b", Expression: "count(*)", Type: runtimev1.MetricsViewSpec_MEASURE_TYPE_SIMPLE}},
	}
	testruntime.RequireResource(t, rt, id, &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindMetricsView, Name: "foobar"},
			Refs:      []*runtimev1.ResourceName{{Kind: runtime.ResourceKindModel, Name: "bar"}},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{"/metrics/foobar.yaml"},
		},
		Resource: &runtimev1.Resource_MetricsView{
			MetricsView: &runtimev1.MetricsViewV2{
				Spec: mvSpec,
				State: &runtimev1.MetricsViewState{
					ValidSpec: &runtimev1.MetricsViewSpec{
						Connector:  "duckdb",
						Model:      "bar",
						Table:      "bar",
						Dimensions: []*runtimev1.MetricsViewSpec_DimensionV2{{Name: "a", Column: "a"}},
						Measures:   []*runtimev1.MetricsViewSpec_MeasureV2{{Name: "b", Expression: "count(*)", Type: runtimev1.MetricsViewSpec_MEASURE_TYPE_SIMPLE}},
					},
				},
			},
		},
	})

	// Add syntax error in source, check downstream resources error
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/sources/foo.yaml": `
connector: local_file
path
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 3, 1)

	// Verify the model (errored and state cleared)
	testruntime.RequireResource(t, rt, id, &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:           &runtimev1.ResourceName{Kind: runtime.ResourceKindModel, Name: "bar"},
			Owner:          runtime.GlobalProjectParserName,
			FilePaths:      []string{"/models/bar.sql"},
			ReconcileError: "Table with name foo does not exist",
		},
		Resource: &runtimev1.Resource_Model{
			Model: &runtimev1.ModelV2{
				Spec: &runtimev1.ModelSpec{
					RefreshSchedule: &runtimev1.Schedule{RefUpdate: true},
					InputConnector:  "duckdb",
					InputProperties: must(structpb.NewStruct(map[string]any{"sql": "SELECT * FROM foo"})),
					OutputConnector: "duckdb",
				},
				State: &runtimev1.ModelState{},
			},
		},
	})
	testruntime.RequireNoOLAPTable(t, rt, id, "bar")

	// Verify the metrics view (errored and state cleared)
	testruntime.RequireResource(t, rt, id, &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:           &runtimev1.ResourceName{Kind: runtime.ResourceKindMetricsView, Name: "foobar"},
			Refs:           []*runtimev1.ResourceName{{Kind: runtime.ResourceKindModel, Name: "bar"}},
			Owner:          runtime.GlobalProjectParserName,
			FilePaths:      []string{"/metrics/foobar.yaml"},
			ReconcileError: "does not exist",
		},
		Resource: &runtimev1.Resource_MetricsView{
			MetricsView: &runtimev1.MetricsViewV2{
				Spec:  mvSpec,
				State: &runtimev1.MetricsViewState{},
			},
		},
	})

	// Fix source, check downstream resources succeed
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
}

func TestSource(t *testing.T) {
	// Add source
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	testruntime.RequireResource(t, rt, id, &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindSource, Name: "foo"},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{"/sources/foo.yaml"},
		},
		Resource: &runtimev1.Resource_Source{
			Source: &runtimev1.SourceV2{
				Spec: &runtimev1.SourceSpec{
					SourceConnector: "local_file",
					SinkConnector:   "duckdb",
					Properties:      must(structpb.NewStruct(map[string]any{"path": "data/foo.csv"})),
					RefreshSchedule: &runtimev1.Schedule{RefUpdate: true},
				},
				State: &runtimev1.SourceState{
					Connector: "duckdb",
					Table:     "foo",
				},
			},
		},
	})
	testruntime.RequireOLAPTable(t, rt, id, "foo")
	testruntime.RequireOLAPTableCount(t, rt, id, "foo", 3)

	// Update underlying data and refresh, verify table is updated
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
`,
	})
	testruntime.RefreshAndWait(t, rt, id, &runtimev1.ResourceName{Kind: runtime.ResourceKindSource, Name: "foo"})
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	testruntime.RequireOLAPTable(t, rt, id, "foo")
	testruntime.RequireOLAPTableCount(t, rt, id, "foo", 1)

	// Delete the underlying table
	olap, release, err := rt.OLAP(context.Background(), id, "")
	require.NoError(t, err)
	err = olap.Exec(context.Background(), &drivers.Statement{Query: "DROP TABLE foo;"})
	require.NoError(t, err)
	release()
	testruntime.RequireNoOLAPTable(t, rt, id, "foo")

	// Reconcile the source and verify the table is added back
	testruntime.ReconcileAndWait(t, rt, id, &runtimev1.ResourceName{Kind: runtime.ResourceKindSource, Name: "foo"})
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	testruntime.RequireOLAPTable(t, rt, id, "foo")
	testruntime.RequireOLAPTableCount(t, rt, id, "foo", 1)

	// Change the source so it errors
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/sources/foo.yaml": `
connector: local_file
path: data/bar.csv
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 1, 0)
	testruntime.RequireNoOLAPTable(t, rt, id, "foo")

	// Restore the source, verify it works again
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	testruntime.RequireResource(t, rt, id, &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindSource, Name: "foo"},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{"/sources/foo.yaml"},
		},
		Resource: &runtimev1.Resource_Source{
			Source: &runtimev1.SourceV2{
				Spec: &runtimev1.SourceSpec{
					SourceConnector: "local_file",
					SinkConnector:   "duckdb",
					Properties:      must(structpb.NewStruct(map[string]any{"path": "data/foo.csv"})),
					RefreshSchedule: &runtimev1.Schedule{RefUpdate: true},
				},
				State: &runtimev1.SourceState{
					Connector: "duckdb",
					Table:     "foo",
				},
			},
		},
	})
	testruntime.RequireOLAPTable(t, rt, id, "foo")
	testruntime.RequireOLAPTableCount(t, rt, id, "foo", 1)
}

func TestSimultaneousDeleteRenameCreate(t *testing.T) {
	// Add bar and foo
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar.sql": `SELECT 10`,
		"/models/foo.sql": `SELECT 20`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)

	// Delete bar, rename foo to foo_two, add bazz
	testruntime.DeleteFiles(t, rt, id,
		"/models/bar.sql",
		"/models/foo.sql",
	)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/foo_two.sql": `SELECT 20`,
		"/models/bazz.sql":    `SELECT 30`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)

	testruntime.RequireNoOLAPTable(t, rt, id, "bar")
	testruntime.RequireOLAPTable(t, rt, id, "foo_two")
	testruntime.RequireOLAPTable(t, rt, id, "bazz")
}

func TestCacheInvalidation(t *testing.T) {
	// Add source and model
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
		"/models/bar.sql": `SELECT * FROM foo`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)
	testruntime.RequireOLAPTableCount(t, rt, id, "bar", 3)

	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar.sql": `SELECT * FROM foo LIMIT`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 1, 1)

	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar.sql": `SELECT * FROM foo LIMIT 1`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)
	testruntime.RequireOLAPTableCount(t, rt, id, "bar", 1) // limit in model should override the limit in the query
}

func TestSourceRefreshSchedule(t *testing.T) {
	// Add source refresh schedule
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
refresh:
  every: 1
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	testruntime.RequireOLAPTableCount(t, rt, id, "foo", 3)

	// update the data file with only 2 rows
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	// no change in data just yet
	testruntime.RequireOLAPTableCount(t, rt, id, "foo", 3)

	// wait to make sure the data is ingested
	time.Sleep(2 * time.Second) // TODO: is there a way to decrease this wait time?
	testruntime.ReconcileParserAndWait(t, rt, id)
	// data has changed
	testruntime.RequireOLAPTableCount(t, rt, id, "foo", 2)
}

func TestSourceAndModelNameCollission(t *testing.T) {
	// Add source for a file
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	testruntime.RequireOLAPTable(t, rt, id, "foo")

	// Create a source with same name within a different folder
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/other_folder/foo.yaml": `
type: source
connector: local_file
path: data/foo.csv
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 1, 1)
	testruntime.RequireOLAPTable(t, rt, id, "foo")

	// Deleting the other file marks the other as valid
	testruntime.DeleteFiles(t, rt, id, "/other_folder/foo.yaml")
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	testruntime.RequireOLAPTable(t, rt, id, "foo")

	// Create a source with same name using `name` annotation
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/sources/foo_1.yaml": `
name: foo
connector: local_file
path: data/foo.csv
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 1, 1)
	testruntime.RequireOLAPTable(t, rt, id, "foo")

	// Deleting the other file marks the other as valid
	testruntime.DeleteFiles(t, rt, id, "/sources/foo_1.yaml")
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	testruntime.RequireOLAPTable(t, rt, id, "foo")

	// Create a model with same name as the source
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/foo.sql": `SELECT 1`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 1, 1)
	// Data is from the source and model did not override it
	testruntime.RequireOLAPTableCount(t, rt, id, "foo", 3)

	// TODO: any other cases?
}

func TestModelMaterialize(t *testing.T) {
	// Add a simple model
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar.sql": `
select 1
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)

	olap, done, err := rt.OLAP(context.Background(), id, "")
	require.NoError(t, err)
	defer done()

	// Assert that the model is a view
	testruntime.RequireIsView(t, olap, "bar", true)

	// Mark the model as materialized
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar.sql": `
-- @materialize: true
select 1
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	// Assert that the model is a table now
	testruntime.RequireIsView(t, olap, "bar", false)

	// Mark the model as not materialized
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar.sql": `
-- @materialize: false
select 1
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	// Assert that the model is back to being a view
	testruntime.RequireIsView(t, olap, "bar", true)
}

func TestModelCTE(t *testing.T) {
	// Create a model that references a source
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
		"/models/bar.sql": `SELECT * FROM foo`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)
	model, modelRes := newModel("SELECT * FROM foo", "bar", "foo")
	testruntime.RequireResource(t, rt, id, modelRes)
	testruntime.RequireOLAPTable(t, rt, id, "bar")

	// Update model to have a CTE with alias different from the source
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar.sql": `with CTEAlias as (select * from foo) select * from CTEAlias`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)
	model.Spec.InputProperties = must(structpb.NewStruct(map[string]any{"sql": `with CTEAlias as (select * from foo) select * from CTEAlias`}))
	testruntime.RequireResource(t, rt, id, modelRes)
	testruntime.RequireOLAPTable(t, rt, id, "bar")

	// Update model to have a CTE with alias same as the source
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar.sql": `with foo as (select * from memory.foo) select * from foo`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)
	model.Spec.InputProperties = must(structpb.NewStruct(map[string]any{"sql": `with foo as (select * from memory.foo) select * from foo`}))
	modelRes.Meta.Refs = []*runtimev1.ResourceName{}
	testruntime.RequireResource(t, rt, id, modelRes)
	// Refs are removed but the model is valid.
	// TODO: is this expected?
	testruntime.RequireOLAPTable(t, rt, id, "bar")
}

func TestRename(t *testing.T) {
	// Rename model A to B and model B to A, verify success
	// Rename model A to B and source B to A, verify success

	// Create source and model
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
		"/models/bar.sql": `SELECT * FROM foo`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)
	model, modelRes := newModel("SELECT * FROM foo", "bar", "foo")
	testruntime.RequireResource(t, rt, id, modelRes)
	testruntime.RequireOLAPTable(t, rt, id, "bar")

	// Rename the model
	testruntime.RenameFile(t, rt, id, "/models/bar.sql", "/models/bar_new.sql")
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)
	modelRes.Meta.Name.Name = "bar_new"
	modelRes.Meta.FilePaths[0] = "/models/bar_new.sql"
	model.State.ResultProperties = must(structpb.NewStruct(map[string]any{"table": "bar_new", "used_model_name": true, "view": true}))
	model.State.ResultTable = "bar_new"
	testruntime.RequireResource(t, rt, id, modelRes)
	testruntime.RequireOLAPTable(t, rt, id, "bar_new")
	testruntime.RequireNoOLAPTable(t, rt, id, "bar")

	// Rename the model to same name but different case
	testruntime.RenameFile(t, rt, id, "/models/bar_new.sql", "/models/Bar_New.sql")
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)
	modelRes.Meta.Name.Name = "Bar_New"
	modelRes.Meta.FilePaths[0] = "/models/Bar_New.sql"
	model.State.ResultProperties = must(structpb.NewStruct(map[string]any{"table": "Bar_New", "used_model_name": true, "view": true}))
	model.State.ResultTable = "Bar_New"
	testruntime.RequireResource(t, rt, id, modelRes)
	testruntime.RequireOLAPTable(t, rt, id, "Bar_New")

	// Create a model referencing the new model name from before
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar_another.sql": `SELECT * FROM Bar_New`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
	_, anotherModelRes := newModel("SELECT * FROM Bar_New", "bar_another", "Bar_New")
	anotherModelRes.Meta.Refs[0].Kind = runtime.ResourceKindModel
	testruntime.RequireResource(t, rt, id, anotherModelRes)
	testruntime.RequireOLAPTable(t, rt, id, "bar_another")

	// Rename the source to the model's name
	testruntime.RenameFile(t, rt, id, "/sources/foo.yaml", "/sources/Bar_New.yaml")
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 1, 1)
	testruntime.RequireOLAPTable(t, rt, id, "Bar_New")
}

func TestRenameToOther(t *testing.T) {
	// Create source and model
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
		"/models/bar1.sql": `SELECT * FROM foo limit 1`,
		"/models/bar2.sql": `SELECT * FROM foo limit 2`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
	testruntime.RequireOLAPTableCount(t, rt, id, "bar1", 1)
	testruntime.RequireOLAPTableCount(t, rt, id, "bar2", 2)

	// Rename model A to B and model B to A, verify success
	testruntime.RenameFile(t, rt, id, "/models/bar2.sql", "/models/bar3.sql")
	testruntime.RenameFile(t, rt, id, "/models/bar1.sql", "/models/bar2.sql")
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
	testruntime.RequireOLAPTableCount(t, rt, id, "bar2", 1)
	testruntime.RequireOLAPTableCount(t, rt, id, "bar3", 2)
}

func TestRenameReconciling(t *testing.T) {
	adbidsPath, err := filepath.Abs("testruntime/testdata/ad_bids/data/AdBids.csv.gz")
	require.NoError(t, err)

	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/sources/foo.yaml": `
connector: local_file
path: ` + adbidsPath,
	})

	// Trigger a reconcile, but don't wait for it to complete
	ctrl, err := rt.Controller(context.Background(), id)
	require.NoError(t, err)
	err = ctrl.Reconcile(context.Background(), runtime.GlobalProjectParserName)
	require.NoError(t, err)

	// Imperfect way to wait until the reconcile is in progress, but not completed (AdBids seems to take about 100ms to ingest).
	// This seems good enough in practice, and if there's a bug, it will at least identify it some of the time!
	time.Sleep(5 * time.Millisecond)

	// Rename the resource while the reconcile is still running
	testruntime.RenameFile(t, rt, id, "/sources/foo.yaml", "/sources/bar.yaml")

	// Wait for it to complete and verify the output is stable
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	testruntime.RequireOLAPTable(t, rt, id, "bar")
}

func TestInterdependence(t *testing.T) {
	// Test D -> C, D -> A, C -> A,B (-> = refs)
	// Test error propagation on source error

	// Create interdependent model
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
		"/models/bar1.sql": `SELECT * FROM foo`,
		"/models/bar2.sql": `SELECT * FROM bar1`,
		"/models/bar3.sql": `SELECT * FROM bar2`,
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar3
dimensions:
- column: b
- column: c
measures:
- expression: count(*)
- expression: avg(a)
`,
	})
	metrics, metricsRes := newMetricsView("dash", "bar3", []string{"count(*)", "avg(a)"}, []string{"b", "c"})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 6, 0, 0)
	testruntime.RequireOLAPTableCount(t, rt, id, "bar1", 3)
	testruntime.RequireOLAPTableCount(t, rt, id, "bar2", 3)
	testruntime.RequireOLAPTableCount(t, rt, id, "bar3", 3)
	testruntime.RequireResource(t, rt, id, metricsRes)

	// Update the source to invalid file
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/sources/foo.yaml": `
connector: local_file
path: data/bar.csv
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 6, 5, 0)
	testruntime.RequireNoOLAPTable(t, rt, id, "bar1")
	testruntime.RequireNoOLAPTable(t, rt, id, "bar2")
	testruntime.RequireNoOLAPTable(t, rt, id, "bar3")
	metricsRes.Meta.ReconcileError = `table "bar3" does not exist`
	metrics.State = &runtimev1.MetricsViewState{}
	testruntime.RequireResource(t, rt, id, metricsRes)
}

func TestCyclesWithTwoModels(t *testing.T) {
	// Create cyclic model
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar1.sql": `SELECT * FROM bar2`,
		"/models/bar2.sql": `SELECT * FROM bar1`,
	})
	bar1Model, bar1Res := newModel("SELECT * FROM bar2", "bar1", "bar2")
	bar1Res.Meta.ReconcileError = `dependency`
	bar1Res.Meta.Refs[0].Kind = runtime.ResourceKindModel
	bar1Model.State = &runtimev1.ModelState{}
	bar2Model, bar2Res := newModel("SELECT * FROM bar1", "bar2", "bar1")
	bar2Res.Meta.ReconcileError = `dependency`
	bar2Res.Meta.Refs[0].Kind = runtime.ResourceKindModel
	bar2Model.State = &runtimev1.ModelState{}
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 2, 0)
	testruntime.RequireResource(t, rt, id, bar1Res)
	testruntime.RequireResource(t, rt, id, bar2Res)

	testruntime.DeleteFiles(t, rt, id, "/models/bar1.sql")
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 1, 0)
	bar2Res.Meta.ReconcileError = `Catalog Error: Table with name bar1 does not exist!`
	bar2Res.Meta.Refs = []*runtimev1.ResourceName{}
	testruntime.RequireResource(t, rt, id, bar2Res)
}

func TestSelfReference(t *testing.T) {
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar1.sql": `SELECT * FROM bar1`,
	})
	bar1Model, bar1Res := newModel("SELECT * FROM bar1", "bar1", "bar1")
	bar1Res.Meta.Refs[0].Kind = runtime.ResourceKindModel
	bar1Res.Meta.ReconcileError = `cyclic dependency`
	bar1Model.State = &runtimev1.ModelState{}

	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 2, 1, 0)
	testruntime.RequireResource(t, rt, id, bar1Res)

	testruntime.DeleteFiles(t, rt, id, "/models/bar1.sql")
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 1, 0, 0)
}

func TestCyclesWithThreeModels(t *testing.T) {
	// Create cyclic model
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
		"/models/bar1.sql": `SELECT * FROM bar2`,
		"/models/bar2.sql": `SELECT * FROM bar3`,
		"/models/bar3.sql": `SELECT * FROM bar1`,
	})
	bar1Model, bar1Res := newModel("SELECT * FROM bar2", "bar1", "bar2")
	bar1Res.Meta.ReconcileError = `dependency`
	bar1Res.Meta.Refs[0].Kind = runtime.ResourceKindModel
	bar1Model.State = &runtimev1.ModelState{}
	bar2Model, bar2Res := newModel("SELECT * FROM bar3", "bar2", "bar3")
	bar2Res.Meta.ReconcileError = `dependency`
	bar2Res.Meta.Refs[0].Kind = runtime.ResourceKindModel
	bar2Model.State = &runtimev1.ModelState{}
	bar3Model, bar3Res := newModel("SELECT * FROM bar1", "bar3", "bar1")
	bar3Res.Meta.ReconcileError = `dependency`
	bar3Res.Meta.Refs[0].Kind = runtime.ResourceKindModel
	bar3Model.State = &runtimev1.ModelState{}
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 5, 3, 0)
	testruntime.RequireResource(t, rt, id, bar1Res)
	testruntime.RequireResource(t, rt, id, bar2Res)
	testruntime.RequireResource(t, rt, id, bar3Res)

	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar1.sql": `SELECT * FROM bar2`,
		"/models/bar2.sql": `SELECT * FROM bar3`,
		"/models/bar3.sql": `SELECT * FROM foo`,
	})
	_, bar1Res = newModel("SELECT * FROM bar2", "bar1", "bar2")
	bar1Res.Meta.Refs[0].Kind = runtime.ResourceKindModel
	_, bar2Res = newModel("SELECT * FROM bar3", "bar2", "bar3")
	bar2Res.Meta.Refs[0].Kind = runtime.ResourceKindModel
	_, bar3Res = newModel("SELECT * FROM foo", "bar3", "foo")
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 5, 0, 0)
	testruntime.RequireResource(t, rt, id, bar1Res)
	testruntime.RequireResource(t, rt, id, bar2Res)
	testruntime.RequireResource(t, rt, id, bar3Res)
}

func TestMetricsView(t *testing.T) {
	// Create source and model
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
		"/models/bar.sql": `SELECT * FROM foo`,
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: b
- column: c
measures:
- expression: count(*)
- expression: avg(a)
`,
	})

	_, metricsRes := newMetricsView("dash", "bar", []string{"count(*)", "avg(a)"}, []string{"b", "c"})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
	testruntime.RequireResource(t, rt, id, metricsRes)

	// ignore invalid measure and dimension
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: b
- column: f
  ignore: true
measures:
- expression: count(*)
- expression: avg(g)
  ignore: true
`,
	})
	_, metricsRes = newMetricsView("dash", "bar", []string{"count(*)"}, []string{"b"})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
	testruntime.RequireResource(t, rt, id, metricsRes)

	// no measure, invalid dashboard
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: b
- column: f
  ignore: true
measures:
- expression: count(*)
  ignore: true
- expression: avg(g)
  ignore: true
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 1, 1)
	testruntime.RequireParseErrors(t, rt, id, map[string]string{"/metrics/dash.yaml": "must define at least one measure"})

	// no dimension. valid dashboard
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: b
  ignore: true
- column: f
  ignore: true
measures:
- expression: count(*)
- expression: avg(g)
  ignore: true
`,
	})
	_, metricsRes = newMetricsView("dash", "bar", []string{"count(*)"}, []string{})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
	testruntime.RequireResource(t, rt, id, metricsRes)

	// duplicate measure name, invalid dashboard
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: b
- column: c
measures:
- expression: count(*)
  name: m
- expression: avg(a)
  name: m
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 1, 1)
	testruntime.RequireParseErrors(t, rt, id, map[string]string{"/metrics/dash.yaml": "found duplicate dimension or measure"})

	// duplicate dimension name, invalid dashboard
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: b
  name: d
- column: c
  name: d
measures:
- expression: count(*)
- expression: avg(a)
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 1, 1)
	testruntime.RequireParseErrors(t, rt, id, map[string]string{"/metrics/dash.yaml": "found duplicate dimension or measure"})

	// duplicate cross name between measures and dimensions, invalid dashboard
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: b
  name: d
- column: c
measures:
- expression: count(*)
  name: d
- expression: avg(a)
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 1, 1)
	testruntime.RequireParseErrors(t, rt, id, map[string]string{"/metrics/dash.yaml": "found duplicate dimension or measure"})

	// reset to valid dashboard
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: b
- column: c
measures:
- expression: count(*)
- expression: avg(a)
`,
	})
	metrics, metricsRes := newMetricsView("dash", "bar", []string{"count(*)", "avg(a)"}, []string{"b", "c"})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
	testruntime.RequireResource(t, rt, id, metricsRes)

	// Model has error, dashboard has error as well
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar.sql": `SELECT * FROM fo`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 2, 0)
	metricsRes.Meta.ReconcileError = `table "bar" does not exist`
	metrics.State = &runtimev1.MetricsViewState{}
	testruntime.RequireResource(t, rt, id, metricsRes)
}

func TestStageChanges(t *testing.T) {
	// Create source and model
	rt, id := testruntime.NewInstanceWithOptions(t, testruntime.InstanceOptions{
		Files:        map[string]string{"rill.yaml": ""},
		StageChanges: true,
	})
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
		"/models/bar.sql": `SELECT * FROM foo`,
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: b
- column: c
measures:
- expression: count(*)
- expression: avg(a)
`,
	})
	_, modelRes := newModel("SELECT * FROM foo", "bar", "foo")
	_, metricsRes := newMetricsView("dash", "bar", []string{"count(*)", "avg(a)"}, []string{"b", "c"})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
	testruntime.RequireResource(t, rt, id, modelRes)
	testruntime.RequireResource(t, rt, id, metricsRes)

	// Invalid source
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/sources/foo.yaml": `
connector: local_file
path: data/bar.csv
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 2, 0)
	// model has error but table is retained
	modelRes.Meta.ReconcileError = "dependency error"
	testruntime.RequireResource(t, rt, id, modelRes)
	testruntime.RequireOLAPTable(t, rt, id, "foo")
	// metrics has no error
	// TODO: is this expected?
	testruntime.RequireResource(t, rt, id, metricsRes)
}

func TestWatch(t *testing.T) {
	// Create instance with watch
	// Create source, wait and verify
	// Create model, wait and verify
	// Create metrics view, wait and verify
	// Drop source, wait and verify

	rt, id := testruntime.NewInstanceWithOptions(t, testruntime.InstanceOptions{
		Files:     map[string]string{"rill.yaml": ""},
		WatchRepo: true,
	})

	ctrl, err := rt.Controller(context.Background(), id)
	require.NoError(t, err)

	// Since we're using WatchRepo, we can't use testruntime.ReconcileParserAndWait.
	// For now, we'll just add a sleep to give the file watcher time to trigger.
	// NOTE: Refactor to wait for the controller to actually be triggered if we ever have instability
	awaitIdle := func() {
		time.Sleep(2 * time.Second)
		err = ctrl.WaitUntilIdle(context.Background(), true)
		require.NoError(t, err)
	}

	// Make sure there's time for the watcher to start
	awaitIdle()

	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
	})
	awaitIdle()
	testruntime.RequireReconcileState(t, rt, id, 2, 0, 0)
	testruntime.RequireOLAPTable(t, rt, id, "foo")
	_, sourceRes := newSource("foo", "data/foo.csv")
	testruntime.RequireResource(t, rt, id, sourceRes)

	testruntime.PutFiles(t, rt, id, map[string]string{
		"/models/bar.sql": `SELECT * FROM foo`,
	})
	awaitIdle()
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)
	testruntime.RequireOLAPTable(t, rt, id, "bar")
	_, modelRes := newModel("SELECT * FROM foo", "bar", "foo")
	testruntime.RequireResource(t, rt, id, modelRes)

	testruntime.PutFiles(t, rt, id, map[string]string{
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: b
- column: c
measures:
- expression: count(*)
- expression: avg(a)
		`,
	})
	awaitIdle()
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
	_, metricsRes := newMetricsView("dash", "bar", []string{"count(*)", "avg(a)"}, []string{"b", "c"})
	testruntime.RequireResource(t, rt, id, metricsRes)
}

func TestExploreTheme(t *testing.T) {
	// Create source and model
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `a,b,c,d,e
1,2,3,4,5
1,2,3,4,5
1,2,3,4,5
`,
		"/sources/foo.yaml": `
type: source
connector: local_file
path: data/foo.csv
`,
		"/models/bar.sql": `SELECT * FROM foo`,
		"/metrics/m1.yaml": `
version: 1
type: metrics_view
model: bar
dimensions:
- column: b
measures:
- expression: count(*)
`,
		"explores/e1.yaml": `
type: explore
metrics_view: m1
display_name: Hello
theme: t1
`,
		"themes/t1.yaml": `
type: theme
colors:
  primary: red
  secondary: grey
`,
	})

	theme := &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindTheme, Name: "t1"},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{"/themes/t1.yaml"},
		},
		Resource: &runtimev1.Resource_Theme{
			Theme: &runtimev1.Theme{
				Spec: &runtimev1.ThemeSpec{
					PrimaryColor: &runtimev1.Color{
						Red:   1,
						Green: 0,
						Blue:  0,
						Alpha: 1,
					},
					SecondaryColor: &runtimev1.Color{
						Red:   0.5019608,
						Green: 0.5019608,
						Blue:  0.5019608,
						Alpha: 1,
					},
					PrimaryColorRaw:   "red",
					SecondaryColorRaw: "grey",
				},
			},
		},
	}

	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 6, 0, 0)
	testruntime.RequireResource(t, rt, id, theme)

	exp := testruntime.GetResource(t, rt, id, runtime.ResourceKindExplore, "e1")
	require.Equal(t, exp.GetExplore().State.ValidSpec.Theme, "t1")
	require.ElementsMatch(t, exp.Meta.Refs, []*runtimev1.ResourceName{
		{Kind: runtime.ResourceKindTheme, Name: "t1"},
		{Kind: runtime.ResourceKindMetricsView, Name: "m1"},
	})

	// make the theme invalid
	testruntime.PutFiles(t, rt, id, map[string]string{
		`themes/t1.yaml`: `
type: theme
colors:
  primary: xxx
  secondary: xxx
`,
	})

	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 5, 2, 1)

	exp = testruntime.GetResource(t, rt, id, runtime.ResourceKindExplore, "e1")
	require.Nil(t, exp.GetExplore().State.ValidSpec)

	// make the theme valid
	testruntime.PutFiles(t, rt, id, map[string]string{
		`themes/t1.yaml`: `
type: theme
colors:
  primary: red
  secondary: grey
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 6, 0, 0)
}

func TestAlert(t *testing.T) {
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"/data/foo.csv": `__time,country
2024-01-01T00:00:00Z,Denmark
`,
		"/sources/foo.yaml": `
connector: local_file
path: data/foo.csv
`,
		"/models/bar.sql": `SELECT * FROM foo`,
		"/metrics/dash.yaml": `
version: 1
type: metrics_view
display_name: dash
model: bar
dimensions:
- column: country
measures:
- expression: count(*)
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)

	_, metricsRes := newMetricsView("dash", "bar", []string{"count(*)"}, []string{"country"})
	testruntime.RequireResource(t, rt, id, metricsRes)
}

func TestExplores(t *testing.T) {
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"models/m1.sql": `SELECT 'foo' as foo, 'bar' as bar, 'int' as internal, 1 as x, 2 as y`,
		"metrics_views/mv1.yaml": `
version: 1
type: metrics_view
model: m1
dimensions:
- column: foo
- column: bar
- column: internal
measures:
- name: x
  expression: sum(x)
- name: y
  expression: sum(y)
security:
  access: true
  row_filter: true
  exclude:
    - if: "{{ not .user.admin }}"
      names: ['internal']
`,
		"explores/e1.yaml": `
type: explore
display_name: Hello
metrics_view: mv1
dimensions:
  exclude: ['internal']
measures: '*'
time_zones: ['UTC', 'America/Los_Angeles']
defaults:
  measures: ['x']
  comparison_mode: time
`,
	})

	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 4, 0, 0)
	testruntime.RequireResource(t, rt, id, &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindExplore, Name: "e1"},
			Refs:      []*runtimev1.ResourceName{{Kind: runtime.ResourceKindMetricsView, Name: "mv1"}},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{"/explores/e1.yaml"},
		},
		Resource: &runtimev1.Resource_Explore{
			Explore: &runtimev1.Explore{
				Spec: &runtimev1.ExploreSpec{
					DisplayName: "Hello",
					MetricsView: "mv1",
					Dimensions:  nil,
					DimensionsSelector: &runtimev1.FieldSelector{
						Invert:   true,
						Selector: &runtimev1.FieldSelector_Fields{Fields: &runtimev1.StringListValue{Values: []string{"internal"}}},
					},
					Measures:         nil,
					MeasuresSelector: &runtimev1.FieldSelector{Selector: &runtimev1.FieldSelector_All{All: true}},
					TimeZones:        []string{"UTC", "America/Los_Angeles"},
					DefaultPreset: &runtimev1.ExplorePreset{
						DimensionsSelector: &runtimev1.FieldSelector{Selector: &runtimev1.FieldSelector_All{All: true}},
						Measures:           []string{"x"},
						ComparisonMode:     runtimev1.ExploreComparisonMode_EXPLORE_COMPARISON_MODE_TIME,
					},
				},
				State: &runtimev1.ExploreState{
					ValidSpec: &runtimev1.ExploreSpec{
						DisplayName: "Hello",
						MetricsView: "mv1",
						Dimensions:  []string{"foo", "bar"},
						Measures:    []string{"x", "y"},
						TimeZones:   []string{"UTC", "America/Los_Angeles"},
						DefaultPreset: &runtimev1.ExplorePreset{
							Dimensions:     []string{"foo", "bar"},
							Measures:       []string{"x"},
							ComparisonMode: runtimev1.ExploreComparisonMode_EXPLORE_COMPARISON_MODE_TIME,
						},
						SecurityRules: []*runtimev1.SecurityRule{
							{Rule: &runtimev1.SecurityRule_Access{Access: &runtimev1.SecurityRuleAccess{
								Condition: "true",
								Allow:     true,
							}}},
							{Rule: &runtimev1.SecurityRule_FieldAccess{FieldAccess: &runtimev1.SecurityRuleFieldAccess{
								Allow:     true,
								AllFields: true,
							}}},
							{Rule: &runtimev1.SecurityRule_FieldAccess{FieldAccess: &runtimev1.SecurityRuleFieldAccess{
								Condition: "{{ not .user.admin }}",
								Allow:     false,
								Fields:    []string{"internal"},
							}}},
						},
					},
				},
			},
		},
	})
}

func newSource(name, path string) (*runtimev1.SourceV2, *runtimev1.Resource) {
	source := &runtimev1.SourceV2{
		Spec: &runtimev1.SourceSpec{
			SourceConnector: "local_file",
			SinkConnector:   "duckdb",
			Properties:      must(structpb.NewStruct(map[string]any{"path": path})),
			RefreshSchedule: &runtimev1.Schedule{RefUpdate: true},
		},
		State: &runtimev1.SourceState{
			Connector: "duckdb",
			Table:     name,
		},
	}
	sourceRes := &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindSource, Name: name},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{fmt.Sprintf("/sources/%s.yaml", name)},
		},
		Resource: &runtimev1.Resource_Source{
			Source: source,
		},
	}
	return source, sourceRes
}

func newModel(query, name, source string) (*runtimev1.ModelV2, *runtimev1.Resource) {
	model := &runtimev1.ModelV2{
		Spec: &runtimev1.ModelSpec{
			RefreshSchedule: &runtimev1.Schedule{RefUpdate: true},
			InputConnector:  "duckdb",
			InputProperties: must(structpb.NewStruct(map[string]any{"sql": query})),
			OutputConnector: "duckdb",
		},
		State: &runtimev1.ModelState{
			ExecutorConnector: "duckdb",
			ResultConnector:   "duckdb",
			ResultProperties:  must(structpb.NewStruct(map[string]any{"table": name, "used_model_name": true, "view": true})),
			ResultTable:       name,
		},
	}
	modelRes := &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindModel, Name: name},
			Refs:      []*runtimev1.ResourceName{{Kind: runtime.ResourceKindSource, Name: source}},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{fmt.Sprintf("/models/%s.sql", name)},
		},
		Resource: &runtimev1.Resource_Model{
			Model: model,
		},
	}
	return model, modelRes
}

func newMetricsView(name, model string, measures, dimensions []string) (*runtimev1.MetricsViewV2, *runtimev1.Resource) {
	metrics := &runtimev1.MetricsViewV2{
		Spec: &runtimev1.MetricsViewSpec{
			Connector:   "duckdb",
			Model:       model,
			DisplayName: name,
			Measures:    make([]*runtimev1.MetricsViewSpec_MeasureV2, len(measures)),
			Dimensions:  make([]*runtimev1.MetricsViewSpec_DimensionV2, len(dimensions)),
		},
		State: &runtimev1.MetricsViewState{
			ValidSpec: &runtimev1.MetricsViewSpec{
				Connector:   "duckdb",
				Table:       model,
				Model:       model,
				DisplayName: name,
				Measures:    make([]*runtimev1.MetricsViewSpec_MeasureV2, len(measures)),
				Dimensions:  make([]*runtimev1.MetricsViewSpec_DimensionV2, len(dimensions)),
			},
		},
	}

	for i, measure := range measures {
		metrics.Spec.Measures[i] = &runtimev1.MetricsViewSpec_MeasureV2{
			Name:       fmt.Sprintf("measure_%d", i),
			Expression: measure,
			Type:       runtimev1.MetricsViewSpec_MEASURE_TYPE_SIMPLE,
		}
		metrics.State.ValidSpec.Measures[i] = &runtimev1.MetricsViewSpec_MeasureV2{
			Name:       fmt.Sprintf("measure_%d", i),
			Expression: measure,
			Type:       runtimev1.MetricsViewSpec_MEASURE_TYPE_SIMPLE,
		}
	}
	for i, dimension := range dimensions {
		metrics.Spec.Dimensions[i] = &runtimev1.MetricsViewSpec_DimensionV2{
			Name:   dimension,
			Column: dimension,
		}
		metrics.State.ValidSpec.Dimensions[i] = &runtimev1.MetricsViewSpec_DimensionV2{
			Name:   dimension,
			Column: dimension,
		}
	}
	metricsRes := &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindMetricsView, Name: name},
			Refs:      []*runtimev1.ResourceName{{Kind: runtime.ResourceKindModel, Name: model}},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{fmt.Sprintf("/metrics/%s.yaml", name)},
		},
		Resource: &runtimev1.Resource_MetricsView{
			MetricsView: metrics,
		},
	}
	return metrics, metricsRes
}

func TestDedicatedConnector(t *testing.T) {
	// Add source, model, and dashboard
	rt, id := testruntime.NewInstance(t)
	testruntime.PutFiles(t, rt, id, map[string]string{
		"rill.yaml": `
connectors:
- name: s3-integrated
  type: s3
`,
		// Dedicated S3 connector
		"/connectors/s3-dedicated.yaml": `
driver: s3
name: s3-dedicated
region: us-west-2
`,
		// Dedicated GCS connector with a custom name
		"/connectors/gcs-dedicated.yaml": `
driver: gcs
name: my-gcs
`,
	})
	testruntime.ReconcileParserAndWait(t, rt, id)
	testruntime.RequireReconcileState(t, rt, id, 3, 0, 0)

	// Verify the dedicated connectors
	testruntime.RequireResource(t, rt, id, &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindConnector, Name: "s3-dedicated"},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{"/connectors/s3-dedicated.yaml"},
		},
		Resource: &runtimev1.Resource_Connector{
			Connector: &runtimev1.ConnectorV2{
				Spec:  &runtimev1.ConnectorSpec{Driver: "s3", Properties: map[string]string{"region": "us-west-2"}},
				State: &runtimev1.ConnectorState{},
			},
		},
	})
	testruntime.RequireResource(t, rt, id, &runtimev1.Resource{
		Meta: &runtimev1.ResourceMeta{
			Name:      &runtimev1.ResourceName{Kind: runtime.ResourceKindConnector, Name: "my-gcs"},
			Owner:     runtime.GlobalProjectParserName,
			FilePaths: []string{"/connectors/gcs-dedicated.yaml"},
		},
		Resource: &runtimev1.Resource_Connector{
			Connector: &runtimev1.ConnectorV2{
				Spec:  &runtimev1.ConnectorSpec{Driver: "gcs"},
				State: &runtimev1.ConnectorState{},
			},
		},
	})
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
