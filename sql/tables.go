// Copyright 2022 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sql

// Table represents the backend of a SQL table.
type Table interface {
	Nameable
	String() string
	Schema() Schema
	Collation() CollationID
	Partitions(*Context) (PartitionIter, error)
	PartitionRows(*Context, Partition) (RowIter, error)
}

// Table2 is an experimental future interface alternative to Table to provide faster access.
type Table2 interface {
	Table

	PartitionRows2(ctx *Context, part Partition) (RowIter2, error)
}

// TableFunction is a node that is generated by a function
type TableFunction interface {
	Node
	Expressioner
	Databaser
	Nameable

	// NewInstance returns a new instance of the table function
	NewInstance(ctx *Context, db Database, expressions []Expression) (Node, error)
}

// TemporaryTable allows tables to declare that they are temporary
type TemporaryTable interface {
	IsTemporary() bool
}

// TableWrapper is a node that wraps the real table. This is needed because
// wrappers cannot implement some methods the table may implement.
type TableWrapper interface {
	// Underlying returns the underlying table.
	Underlying() Table
}

// FilteredTable is a table that can produce a specific RowIter
// that's more optimized given the filters.
type FilteredTable interface {
	Table
	Filters() []Expression
	HandledFilters(filters []Expression) []Expression
	WithFilters(ctx *Context, filters []Expression) Table
}

// ProjectedTable is a table that can produce a specific RowIter
// that's more optimized given the columns that are projected.
type ProjectedTable interface {
	Table
	WithProjections(colNames []string) Table
	Projections() []string
}

// IndexAddressable is a table that can be scanned through a primary index
type IndexAddressable interface {
	// IndexedAccess returns a table that can perform scans constrained to
	// an IndexLookup on the index given
	IndexedAccess(Index) IndexedTable
	// GetIndexes returns an array of this table's Indexes
	GetIndexes(ctx *Context) ([]Index, error)
}

// IndexAddressableTable is a table that can be accessed through an index
type IndexAddressableTable interface {
	Table
	IndexAddressable
}

// IndexedTable is a table with an index chosen for range scans
type IndexedTable interface {
	Table
	// LookupPartitions returns partitions scanned by the given IndexLookup
	LookupPartitions(*Context, IndexLookup) (PartitionIter, error)
}

// IndexAlterableTable represents a table that supports index modification operations.
type IndexAlterableTable interface {
	Table
	// CreateIndex creates an index for this table, using the provided parameters.
	// Returns an error if the index name already exists, or an index with the same columns already exists.
	CreateIndex(ctx *Context, indexDef IndexDef) error
	// DropIndex removes an index from this table, if it exists.
	// Returns an error if the removal failed or the index does not exist.
	DropIndex(ctx *Context, indexName string) error
	// RenameIndex renames an existing index to another name that is not already taken by another index on this table.
	RenameIndex(ctx *Context, fromIndexName string, toIndexName string) error
}

// ForeignKeyTable is a table that can declare its foreign key constraints, as well as be referenced.
type ForeignKeyTable interface {
	IndexAddressableTable
	// CreateIndexForForeignKey creates an index for this table, using the provided parameters. Indexes created through
	// this function are specifically ones generated for use with a foreign key. Returns an error if the index name
	// already exists, or an index on the same columns already exists.
	CreateIndexForForeignKey(ctx *Context, indexDef IndexDef) error

	// GetDeclaredForeignKeys returns the foreign key constraints that are declared by this table.
	GetDeclaredForeignKeys(ctx *Context) ([]ForeignKeyConstraint, error)
	// GetReferencedForeignKeys returns the foreign key constraints that are referenced by this table.
	GetReferencedForeignKeys(ctx *Context) ([]ForeignKeyConstraint, error)
	// AddForeignKey adds the given foreign key constraint to the table. Returns an error if the foreign key name
	// already exists on any other table within the database.
	AddForeignKey(ctx *Context, fk ForeignKeyConstraint) error
	// DropForeignKey removes a foreign key from the table.
	DropForeignKey(ctx *Context, fkName string) error
	// UpdateForeignKey updates the given foreign key constraint. May range from updated table names to setting the
	// IsResolved boolean.
	UpdateForeignKey(ctx *Context, fkName string, fk ForeignKeyConstraint) error
	// GetForeignKeyEditor returns a ForeignKeyEditor for this table.
	GetForeignKeyEditor(ctx *Context) ForeignKeyEditor
}

// CheckTable is a table that can declare its check constraints.
type CheckTable interface {
	Table
	// GetChecks returns the check constraints on this table.
	GetChecks(ctx *Context) ([]CheckDefinition, error)
}

// CheckAlterableTable represents a table that supports check constraints.
type CheckAlterableTable interface {
	Table
	// CreateCheck creates an check constraint for this table, using the provided parameters.
	// Returns an error if the constraint name already exists.
	CreateCheck(ctx *Context, check *CheckDefinition) error
	// DropCheck removes a check constraint from the database.
	DropCheck(ctx *Context, chName string) error
}

// PrimaryKeyAlterableTable represents a table that supports primary key changes.
type PrimaryKeyAlterableTable interface {
	Table
	// CreatePrimaryKey creates a primary key for this table, using the provided parameters.
	// Returns an error if the new primary key set is not compatible with the current table data.
	CreatePrimaryKey(ctx *Context, columns []IndexColumn) error
	// DropPrimaryKey drops a primary key on a table. Returns an error if that table does not have a key.
	DropPrimaryKey(ctx *Context) error
}

type PrimaryKeyTable interface {
	// PrimaryKeySchema returns this table's PrimaryKeySchema
	PrimaryKeySchema() PrimaryKeySchema
}

// TableEditor is the base interface for sub interfaces that can update rows in a
// table during an INSERT, REPLACE, UPDATE, or DELETE statement.
type TableEditor interface {
	RowReplacer
	RowUpdater
}

type EditOpenerCloser interface {
	// StatementBegin is called before the first operation of a statement. Integrators should mark the state of the data
	// in some way that it may be returned to in the case of an error.
	StatementBegin(ctx *Context)
	// DiscardChanges is called if a statement encounters an error, and all current changes since the statement beginning
	// should be discarded.
	DiscardChanges(ctx *Context, errorEncountered error) error
	// StatementComplete is called after the last operation of the statement, indicating that it has successfully completed.
	// The mark set in StatementBegin may be removed, and a new one should be created on the next StatementBegin.
	StatementComplete(ctx *Context) error
}

type AutoIncrementEditor interface {
	AutoIncrementSetter
	AutoIncrementGetter
}

// InsertableTable is a table that can process insertion of new rows.
type InsertableTable interface {
	Table
	// Inserter returns an Inserter for this table. The Inserter will get one call to Insert() for each row to be
	// inserted, and will end with a call to Close() to finalize the insert operation.
	Inserter(*Context) RowInserter
}

// RowInserter is an insert cursor that can insert one or more values to a table.
type RowInserter interface {
	EditOpenerCloser
	// Insert inserts the row given, returning an error if it cannot. Insert will be called once for each row to process
	// for the insert operation, which may involve many rows. After all rows in an operation have been processed, Close
	// is called.
	Insert(*Context, Row) error
	// Close finalizes the insert operation, persisting its result.
	Closer
}

// DeleteableTable is a table that can process the deletion of rows
type DeletableTable interface {
	Table
	// Deleter returns a RowDeleter for this table. The RowDeleter will get one call to Delete for each row to be deleted,
	// and will end with a call to Close() to finalize the delete operation.
	Deleter(*Context) RowDeleter
}

// RowDeleter is a delete cursor that can delete one or more rows from a table.
type RowDeleter interface {
	EditOpenerCloser
	// Delete deletes the given row. Returns ErrDeleteRowNotFound if the row was not found. Delete will be called once for
	// each row to process for the delete operation, which may involve many rows. After all rows have been processed,
	// Close is called.
	Delete(*Context, Row) error
	// Close finalizes the delete operation, persisting the result.
	Closer
}

// TruncateableTable is a table that can process the deletion of all rows.
type TruncateableTable interface {
	Table
	// Truncate removes all rows from the table. If the table also implements DeletableTable and it is determined that
	// truncate would be equivalent to a DELETE which spans the entire table, then this function will be called instead.
	// Returns the number of rows that were removed.
	Truncate(*Context) (int, error)
}

// AutoIncrementTable is a table that supports AUTO_INCREMENT.
// Getter and Setter methods access the table's AUTO_INCREMENT
// sequence. These methods should only be used for tables with
// and AUTO_INCREMENT column in their schema.
type AutoIncrementTable interface {
	Table
	// GetNextAutoIncrementValue gets the next AUTO_INCREMENT value. In the case that a table with an autoincrement
	// column is passed in a row with the autoinc column failed, the next auto increment value must
	// update its internal state accordingly and use the insert val at runtime.
	// Implementations are responsible for updating their state to provide the correct values.
	GetNextAutoIncrementValue(ctx *Context, insertVal interface{}) (uint64, error)
	// AutoIncrementSetter returns an AutoIncrementSetter.
	AutoIncrementSetter(*Context) AutoIncrementSetter
}

// AutoIncrementSetter provides support for altering a table's
// AUTO_INCREMENT sequence, eg 'ALTER TABLE t AUTO_INCREMENT = 10;'
type AutoIncrementSetter interface {
	// SetAutoIncrementValue sets a new AUTO_INCREMENT value.
	SetAutoIncrementValue(*Context, uint64) error
	// Close finalizes the set operation, persisting the result.
	Closer
}

type AutoIncrementGetter interface {
	GetNextAutoIncrementValue(ctx *Context, insertVal interface{}) (uint64, error)
	// Close finalizes the set operation, persisting the result.
	Closer
}

// RowReplacer is a combination of RowDeleter and RowInserter.
type RowReplacer interface {
	EditOpenerCloser
	RowInserter
	RowDeleter
}

// ReplaceableTable allows rows to be replaced through a Delete (if applicable) then Insert.
type ReplaceableTable interface {
	Table
	// Replacer returns a RowReplacer for this table. The RowReplacer will have Insert and optionally Delete called once
	// for each row, followed by a call to Close() when all rows have been processed.
	Replacer(ctx *Context) RowReplacer
}

// UpdatableTable is a table that can process updates of existing rows via update statements.
type UpdatableTable interface {
	Table
	// Updater returns a RowUpdater for this table. The RowUpdater will have Update called once for each row to be
	// updated, followed by a call to Close() when all rows have been processed.
	Updater(ctx *Context) RowUpdater
}

// RowUpdater is an update cursor that can update one or more rows in a table.
type RowUpdater interface {
	EditOpenerCloser
	// Update the given row. Provides both the old and new rows.
	Update(ctx *Context, old Row, new Row) error
	// Closer finalizes the update operation, persisting the result.
	Closer
}

// RewritableTable is an extension to Table that makes it simpler for integrators to adapt to schema changes that must
// rewrite every row of the table. In this case, rows are streamed from the existing table in the old schema,
// transformed / updated appropriately, and written with the new format.
type RewritableTable interface {
	Table
	AlterableTable

	// ShouldRewriteTable returns whether this table should be rewritten because of a schema change. The old and new
	// versions of the schema and modified column are provided. For some operations, one or both of |oldColumn| or
	// |newColumn| may be nil.
	// The engine may decide to rewrite tables regardless in some cases, such as when a new non-nullable column is added.
	ShouldRewriteTable(ctx *Context, oldSchema, newSchema PrimaryKeySchema, oldColumn, newColumn *Column) bool

	// RewriteInserter returns a RowInserter for the new schema. Rows from the current table, with the old schema, will
	// be streamed from the table and passed to this RowInserter. Implementor tables must still return rows in the
	// current schema until the rewrite operation completes. |Close| will be called on RowInserter when all rows have
	// been inserted.
	RewriteInserter(ctx *Context, oldSchema, newSchema PrimaryKeySchema, oldColumn, newColumn *Column, idxCols []IndexColumn) (RowInserter, error)
}

// UnresolvedTable is a Table that is either unresolved or deferred for until an asOf resolution
type UnresolvedTable interface {
	Nameable
	// Database returns the database name
	Database() string
	// WithAsOf returns a copy of this versioned table with its AsOf
	// field set to the given value. Analogous to WithChildren.
	WithAsOf(asOf Expression) (Node, error)
	// AsOf returns this table's asof expression.
	AsOf() Expression
}

// AlterableTable should be implemented by tables that can receive ALTER TABLE statements to modify their schemas.
type AlterableTable interface {
	Table
	UpdatableTable

	// AddColumn adds a column to this table as given. If non-nil, order specifies where in the schema to add the column.
	AddColumn(ctx *Context, column *Column, order *ColumnOrder) error
	// DropColumn drops the column with the name given.
	DropColumn(ctx *Context, columnName string) error
	// ModifyColumn modifies the column with the name given, replacing with the new column definition provided (which may
	// include a name change). If non-nil, order specifies where in the schema to move the column.
	ModifyColumn(ctx *Context, columnName string, column *Column, order *ColumnOrder) error
}

// ForeignKeyEditor is a TableEditor that is addressable via IndexLookup.
type ForeignKeyEditor interface {
	TableEditor
	IndexAddressable
}

