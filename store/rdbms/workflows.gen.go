package rdbms

// This file is an auto-generated file
//
// Template:    pkg/codegen/assets/store_rdbms.gen.go.tpl
// Definitions: store/workflows.yaml
//
// Changes to this file may cause incorrect behavior
// and will be lost if the code is regenerated.

import (
	"context"
	"database/sql"
	"github.com/Masterminds/squirrel"
	"github.com/cortezaproject/corteza-server/pkg/errors"
	"github.com/cortezaproject/corteza-server/pkg/filter"
	"github.com/cortezaproject/corteza-server/store"
	"github.com/cortezaproject/corteza-server/store/rdbms/builders"
	"github.com/cortezaproject/corteza-server/system/types"
)

var _ = errors.Is

// SearchWorkflows returns all matching rows
//
// This function calls convertWorkflowFilter with the given
// types.WorkflowFilter and expects to receive a working squirrel.SelectBuilder
func (s Store) SearchWorkflows(ctx context.Context, f types.WorkflowFilter) (types.WorkflowSet, types.WorkflowFilter, error) {
	var (
		err error
		set []*types.Workflow
		q   squirrel.SelectBuilder
	)

	return set, f, func() error {
		q = s.workflowsSelectBuilder()

		// Paging enabled
		// {search: {enablePaging:true}}
		// Cleanup unwanted cursor values (only relevant is f.PageCursor, next&prev are reset and returned)
		f.PrevPage, f.NextPage = nil, nil

		if f.PageCursor != nil {
			// Page cursor exists so we need to validate it against used sort
			// To cover the case when paging cursor is set but sorting is empty, we collect the sorting instructions
			// from the cursor.
			// This (extracted sorting info) is then returned as part of response
			if f.Sort, err = f.PageCursor.Sort(f.Sort); err != nil {
				return err
			}
		}

		// Make sure results are always sorted at least by primary keys
		if f.Sort.Get("id") == nil {
			f.Sort = append(f.Sort, &filter.SortExpr{
				Column:     "id",
				Descending: f.Sort.LastDescending(),
			})
		}

		// Cloned sorting instructions for the actual sorting
		// Original are passed to the fetchFullPageOfUsers fn used for cursor creation so it MUST keep the initial
		// direction information
		sort := f.Sort.Clone()

		// When cursor for a previous page is used it's marked as reversed
		// This tells us to flip the descending flag on all used sort keys
		if f.PageCursor != nil && f.PageCursor.ROrder {
			sort.Reverse()
		}

		// Apply sorting expr from filter to query
		if q, err = setOrderBy(q, sort, s.sortableWorkflowColumns()); err != nil {
			return err
		}

		set, f.PrevPage, f.NextPage, err = s.fetchFullPageOfWorkflows(
			ctx,
			q, f.Sort, f.PageCursor,
			f.Limit,
			f.Check,
			func(cur *filter.PagingCursor) squirrel.Sqlizer {
				return builders.CursorCondition(cur, nil)
			},
		)

		if err != nil {
			return err
		}

		f.PageCursor = nil
		return nil
	}()
}

// fetchFullPageOfWorkflows collects all requested results.
//
// Function applies:
//  - cursor conditions (where ...)
//  - limit
//
// Main responsibility of this function is to perform additional sequential queries in case when not enough results
// are collected due to failed check on a specific row (by check fn).
//
// Function then moves cursor to the last item fetched
func (s Store) fetchFullPageOfWorkflows(
	ctx context.Context,
	q squirrel.SelectBuilder,
	sort filter.SortExprSet,
	cursor *filter.PagingCursor,
	reqItems uint,
	check func(*types.Workflow) (bool, error),
	cursorCond func(*filter.PagingCursor) squirrel.Sqlizer,
) (set []*types.Workflow, prev, next *filter.PagingCursor, err error) {
	var (
		aux []*types.Workflow

		// When cursor for a previous page is used it's marked as reversed
		// This tells us to flip the descending flag on all used sort keys
		reversedOrder = cursor != nil && cursor.ROrder

		// copy of the select builder
		tryQuery squirrel.SelectBuilder

		// Copy no. of required items to limit
		// Limit will change when doing subsequent queries to fill
		// the set with all required items
		limit = reqItems

		// cursor to prev. page is only calculated when cursor is used
		hasPrev = cursor != nil

		// next cursor is calculated when there are more pages to come
		hasNext bool
	)

	set = make([]*types.Workflow, 0, DefaultSliceCapacity)

	for try := 0; try < MaxRefetches; try++ {
		if cursor != nil {
			tryQuery = q.Where(cursorCond(cursor))
		} else {
			tryQuery = q
		}

		if limit > 0 {
			// fetching + 1 so we know if there are more items
			// we can fetch (next-page cursor)
			tryQuery = tryQuery.Limit(uint64(limit + 1))
		}

		if aux, err = s.QueryWorkflows(ctx, tryQuery, check); err != nil {
			return nil, nil, nil, err
		}

		if len(aux) == 0 {
			// nothing fetched
			break
		}

		// append fetched items
		set = append(set, aux...)

		if reqItems == 0 {
			// no max requested items specified, break out
			break
		}

		collected := uint(len(set))

		if reqItems > collected {
			// not enough items fetched, try again with adjusted limit
			limit = reqItems - collected

			if limit < MinEnsureFetchLimit {
				// In case limit is set very low and we've missed records in the first fetch,
				// make sure next fetch limit is a bit higher
				limit = MinEnsureFetchLimit
			}

			// Update cursor so that it points to the last item fetched
			cursor = s.collectWorkflowCursorValues(set[collected-1], sort...)

			// Copy reverse flag from sorting
			cursor.LThen = sort.Reversed()
			continue
		}

		if reqItems < collected {
			set = set[:reqItems]
			hasNext = true
		}

		break
	}

	collected := len(set)

	if collected == 0 {
		return nil, nil, nil, nil
	}

	if reversedOrder {
		// Fetched set needs to be reversed because we've forced a descending order to get the previous page
		for i, j := 0, collected-1; i < j; i, j = i+1, j-1 {
			set[i], set[j] = set[j], set[i]
		}

		// when in reverse-order rules on what cursor to return change
		hasPrev, hasNext = hasNext, hasPrev
	}

	if hasPrev {
		prev = s.collectWorkflowCursorValues(set[0], sort...)
		prev.ROrder = true
		prev.LThen = !sort.Reversed()
	}

	if hasNext {
		next = s.collectWorkflowCursorValues(set[collected-1], sort...)
		next.LThen = sort.Reversed()
	}

	return set, prev, next, nil
}

// QueryWorkflows queries the database, converts and checks each row and
// returns collected set
//
// Fn also returns total number of fetched items and last fetched item so that the caller can construct cursor
// for next page of results
func (s Store) QueryWorkflows(
	ctx context.Context,
	q squirrel.Sqlizer,
	check func(*types.Workflow) (bool, error),
) ([]*types.Workflow, error) {
	var (
		set = make([]*types.Workflow, 0, DefaultSliceCapacity)
		res *types.Workflow

		// Query rows with
		rows, err = s.Query(ctx, q)
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()
	for rows.Next() {
		if err = rows.Err(); err == nil {
			res, err = s.internalWorkflowRowScanner(rows)
		}

		if err != nil {
			return nil, err
		}

		// check fn set, call it and see if it passed the test
		// if not, skip the item
		if check != nil {
			if chk, err := check(res); err != nil {
				return nil, err
			} else if !chk {
				continue
			}
		}

		set = append(set, res)
	}

	return set, rows.Err()
}

// LookupWorkflowByID searches for workflow by ID
//
// It returns workflow even if deleted or suspended
func (s Store) LookupWorkflowByID(ctx context.Context, id uint64) (*types.Workflow, error) {
	return s.execLookupWorkflow(ctx, squirrel.Eq{
		s.preprocessColumn("wf.id", ""): store.PreprocessValue(id, ""),
	})
}

// LookupWorkflowByHandle searches for workflow by their handle
//
// It returns only valid workflows (not deleted, not suspended)
func (s Store) LookupWorkflowByHandle(ctx context.Context, handle string) (*types.Workflow, error) {
	return s.execLookupWorkflow(ctx, squirrel.Eq{
		s.preprocessColumn("wf.handle", "lower"): store.PreprocessValue(handle, "lower"),

		"wf.deleted_at": nil,
	})
}

// CreateWorkflow creates one or more rows in workflows table
func (s Store) CreateWorkflow(ctx context.Context, rr ...*types.Workflow) (err error) {
	for _, res := range rr {
		err = s.checkWorkflowConstraints(ctx, res)
		if err != nil {
			return err
		}

		err = s.execCreateWorkflows(ctx, s.internalWorkflowEncoder(res))
		if err != nil {
			return err
		}
	}

	return
}

// UpdateWorkflow updates one or more existing rows in workflows
func (s Store) UpdateWorkflow(ctx context.Context, rr ...*types.Workflow) error {
	return s.partialWorkflowUpdate(ctx, nil, rr...)
}

// partialWorkflowUpdate updates one or more existing rows in workflows
func (s Store) partialWorkflowUpdate(ctx context.Context, onlyColumns []string, rr ...*types.Workflow) (err error) {
	for _, res := range rr {
		err = s.checkWorkflowConstraints(ctx, res)
		if err != nil {
			return err
		}

		err = s.execUpdateWorkflows(
			ctx,
			squirrel.Eq{
				s.preprocessColumn("wf.id", ""): store.PreprocessValue(res.ID, ""),
			},
			s.internalWorkflowEncoder(res).Skip("id").Only(onlyColumns...))
		if err != nil {
			return err
		}
	}

	return
}

// UpsertWorkflow updates one or more existing rows in workflows
func (s Store) UpsertWorkflow(ctx context.Context, rr ...*types.Workflow) (err error) {
	for _, res := range rr {
		err = s.checkWorkflowConstraints(ctx, res)
		if err != nil {
			return err
		}

		err = s.execUpsertWorkflows(ctx, s.internalWorkflowEncoder(res))
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteWorkflow Deletes one or more rows from workflows table
func (s Store) DeleteWorkflow(ctx context.Context, rr ...*types.Workflow) (err error) {
	for _, res := range rr {

		err = s.execDeleteWorkflows(ctx, squirrel.Eq{
			s.preprocessColumn("wf.id", ""): store.PreprocessValue(res.ID, ""),
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteWorkflowByID Deletes row from the workflows table
func (s Store) DeleteWorkflowByID(ctx context.Context, ID uint64) error {
	return s.execDeleteWorkflows(ctx, squirrel.Eq{
		s.preprocessColumn("wf.id", ""): store.PreprocessValue(ID, ""),
	})
}

// TruncateWorkflows Deletes all rows from the workflows table
func (s Store) TruncateWorkflows(ctx context.Context) error {
	return s.Truncate(ctx, s.workflowTable())
}

// execLookupWorkflow prepares Workflow query and executes it,
// returning types.Workflow (or error)
func (s Store) execLookupWorkflow(ctx context.Context, cnd squirrel.Sqlizer) (res *types.Workflow, err error) {
	var (
		row rowScanner
	)

	row, err = s.QueryRow(ctx, s.workflowsSelectBuilder().Where(cnd))
	if err != nil {
		return
	}

	res, err = s.internalWorkflowRowScanner(row)
	if err != nil {
		return
	}

	return res, nil
}

// execCreateWorkflows updates all matched (by cnd) rows in workflows with given data
func (s Store) execCreateWorkflows(ctx context.Context, payload store.Payload) error {
	return s.Exec(ctx, s.InsertBuilder(s.workflowTable()).SetMap(payload))
}

// execUpdateWorkflows updates all matched (by cnd) rows in workflows with given data
func (s Store) execUpdateWorkflows(ctx context.Context, cnd squirrel.Sqlizer, set store.Payload) error {
	return s.Exec(ctx, s.UpdateBuilder(s.workflowTable("wf")).Where(cnd).SetMap(set))
}

// execUpsertWorkflows inserts new or updates matching (by-primary-key) rows in workflows with given data
func (s Store) execUpsertWorkflows(ctx context.Context, set store.Payload) error {
	upsert, err := s.config.UpsertBuilder(
		s.config,
		s.workflowTable(),
		set,
		s.preprocessColumn("id", ""),
	)

	if err != nil {
		return err
	}

	return s.Exec(ctx, upsert)
}

// execDeleteWorkflows Deletes all matched (by cnd) rows in workflows with given data
func (s Store) execDeleteWorkflows(ctx context.Context, cnd squirrel.Sqlizer) error {
	return s.Exec(ctx, s.DeleteBuilder(s.workflowTable("wf")).Where(cnd))
}

func (s Store) internalWorkflowRowScanner(row rowScanner) (res *types.Workflow, err error) {
	res = &types.Workflow{}

	if _, has := s.config.RowScanners["workflow"]; has {
		scanner := s.config.RowScanners["workflow"].(func(_ rowScanner, _ *types.Workflow) error)
		err = scanner(row, res)
	} else {
		err = row.Scan(
			&res.ID,
			&res.Handle,
			&res.Meta,
			&res.Enabled,
			&res.Trace,
			&res.KeepSessions,
			&res.Scope,
			&res.Steps,
			&res.Paths,
			&res.RunAs,
			&res.OwnedBy,
			&res.CreatedBy,
			&res.UpdatedBy,
			&res.DeletedBy,
			&res.CreatedAt,
			&res.UpdatedAt,
			&res.DeletedAt,
		)
	}

	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound.Stack(1)
	}

	if err != nil {
		return nil, errors.Store("could not scan workflow db row").Wrap(err)
	} else {
		return res, nil
	}
}

// QueryWorkflows returns squirrel.SelectBuilder with set table and all columns
func (s Store) workflowsSelectBuilder() squirrel.SelectBuilder {
	return s.SelectBuilder(s.workflowTable("wf"), s.workflowColumns("wf")...)
}

// workflowTable name of the db table
func (Store) workflowTable(aa ...string) string {
	var alias string
	if len(aa) > 0 {
		alias = " AS " + aa[0]
	}

	return "workflows" + alias
}

// WorkflowColumns returns all defined table columns
//
// With optional string arg, all columns are returned aliased
func (Store) workflowColumns(aa ...string) []string {
	var alias string
	if len(aa) > 0 {
		alias = aa[0] + "."
	}

	return []string{
		alias + "id",
		alias + "handle",
		alias + "meta",
		alias + "enabled",
		alias + "trace",
		alias + "keep_sessions",
		alias + "scope",
		alias + "steps",
		alias + "paths",
		alias + "run_as",
		alias + "owned_by",
		alias + "created_by",
		alias + "updated_by",
		alias + "deleted_by",
		alias + "created_at",
		alias + "updated_at",
		alias + "deleted_at",
	}
}

// {true true false true true true}

// sortableWorkflowColumns returns all Workflow columns flagged as sortable
//
// With optional string arg, all columns are returned aliased
func (Store) sortableWorkflowColumns() map[string]string {
	return map[string]string{
		"id": "id",
	}
}

// internalWorkflowEncoder encodes fields from types.Workflow to store.Payload (map)
//
// Encoding is done by using generic approach or by calling encodeWorkflow
// func when rdbms.customEncoder=true
func (s Store) internalWorkflowEncoder(res *types.Workflow) store.Payload {
	return store.Payload{
		"id":            res.ID,
		"handle":        res.Handle,
		"meta":          res.Meta,
		"enabled":       res.Enabled,
		"trace":         res.Trace,
		"keep_sessions": res.KeepSessions,
		"scope":         res.Scope,
		"steps":         res.Steps,
		"paths":         res.Paths,
		"run_as":        res.RunAs,
		"owned_by":      res.OwnedBy,
		"created_by":    res.CreatedBy,
		"updated_by":    res.UpdatedBy,
		"deleted_by":    res.DeletedBy,
		"created_at":    res.CreatedAt,
		"updated_at":    res.UpdatedAt,
		"deleted_at":    res.DeletedAt,
	}
}

// collectWorkflowCursorValues collects values from the given resource that and sets them to the cursor
// to be used for pagination
//
// Values that are collected must come from sortable, unique or primary columns/fields
// At least one of the collected columns must be flagged as unique, otherwise fn appends primary keys at the end
//
// Known issue:
//   when collecting cursor values for query that sorts by unique column with partial index (ie: unique handle on
//   undeleted items)
func (s Store) collectWorkflowCursorValues(res *types.Workflow, cc ...*filter.SortExpr) *filter.PagingCursor {
	var (
		cursor = &filter.PagingCursor{}

		hasUnique bool

		// All known primary key columns

		pkId bool

		collect = func(cc ...*filter.SortExpr) {
			for _, c := range cc {
				switch c.Column {
				case "id":
					cursor.Set(c.Column, res.ID, c.Descending)

					pkId = true
				case "handle":
					cursor.Set(c.Column, res.Handle, c.Descending)
					hasUnique = true

				}
			}
		}
	)

	collect(cc...)
	if !hasUnique || !(pkId && true) {
		collect(&filter.SortExpr{Column: "id", Descending: false})
	}

	return cursor
}

// checkWorkflowConstraints performs lookups (on valid) resource to check if any of the values on unique fields
// already exists in the store
//
// Using built-in constraint checking would be more performant but unfortunately we can not rely
// on the full support (MySQL does not support conditional indexes)
func (s *Store) checkWorkflowConstraints(ctx context.Context, res *types.Workflow) error {
	// Consider resource valid when all fields in unique constraint check lookups
	// have valid (non-empty) value
	//
	// Only string and uint64 are supported for now
	// feel free to add additional types if needed
	var valid = true

	valid = valid && len(res.Handle) > 0

	if !valid {
		return nil
	}

	{
		ex, err := s.LookupWorkflowByHandle(ctx, res.Handle)
		if err == nil && ex != nil && ex.ID != res.ID {
			return store.ErrNotUnique.Stack(1)
		} else if !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
