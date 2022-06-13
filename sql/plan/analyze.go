package plan

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/mysql_db"
	"github.com/dolthub/go-mysql-server/sql/transform"
)

type Analyze struct {
	db   sql.Database
	tbls []sql.Node
}

func NewAnalyze(db sql.Database, tbls []sql.Node) *Analyze {
	return &Analyze{
		db:   db,
		tbls: tbls,
	}
}

// Schema implements the interface sql.Node.
// TODO: should be |Tables|Op|Msg_type|Msg_text|
func (n *Analyze) Schema() sql.Schema {
	return sql.OkResultSchema
}

// String implements the interface sql.Node.
func (n *Analyze) String() string {
	tblNames := make([]string, len(n.tbls))
	for i, tbl := range n.tbls {
		switch resTbl := tbl.(type) {
		case *ResolvedTable:
			tblNames[i] = resTbl.Name()
		case *UnresolvedTable:
			tblNames[i] = resTbl.Name()
		case *Exchange:
			tblNames[i] = resTbl.Child.String()
		}
	}
	return fmt.Sprintf("Analyze table %s", strings.Join(tblNames, ", "))
}

// Database implements the interface sql.Databaser.
func (n *Analyze) Database() sql.Database {
	return n.db
}

// WithDatabase implements the interface sql.Databaser.
func (n *Analyze) WithDatabase(db sql.Database) (sql.Node, error) {
	nn := *n
	nn.db = db
	return &nn, nil
}

// Resolved implements the Resolvable interface.
func (n *Analyze) Resolved() bool {
	_, ok := n.db.(sql.UnresolvedDatabase)
	for _, tbl := range n.tbls {
		if !tbl.Resolved() {
			return false
		}
	}
	return !ok
}

// Children implements the interface sql.Node.
func (n *Analyze) Children() []sql.Node {
	return n.tbls
}

// WithChildren implements the interface sql.Node.
func (n *Analyze) WithChildren(children ...sql.Node) (sql.Node, error) {
	// Deep copy children
	newChildren := make([]sql.Node, len(children))
	copy(newChildren, children)

	nn := *n
	nn.tbls = newChildren
	return &nn, nil
}

// CheckPrivileges implements the interface sql.Node.
func (n *Analyze) CheckPrivileges(ctx *sql.Context, opChecker sql.PrivilegedOperationChecker) bool {
	return true
}

// RowIter implements the interface sql.Node.
func (n *Analyze) RowIter(ctx *sql.Context, row sql.Row) (sql.RowIter, error) {
	// Assume table is in current database
	database := ctx.GetCurrentDatabase()
	if database == "" {
		return nil, sql.ErrNoDatabaseSelected.New()
	}

	mysql, ok := n.db.(*mysql_db.MySQLDb)
	if !ok {
		return nil, sql.ErrDatabaseNotFound.New("mysql")
	}
	colStatsTableData := mysql.ColumnStatisticsTable().Data()

	for _, tbl := range n.tbls {
		var resTbl sql.StatisticsTable
		// TODO: find statistics table
		transform.Inspect(tbl, func(n sql.Node) bool {
			if statsTbl, ok := n.(sql.StatisticsTable); ok {
				resTbl = statsTbl
				return false
			}
			return true
		})

		// skip if you couldn't find statistics table (shouldn't be possible)
		if resTbl == nil {
			continue
		}

		resTbl.CalculateStatistics(ctx)

		resTbl.Partitions(ctx)
		resTbl.PartitionRows()

		// Calculate stats
		tblIter, err := resTbl.RowIter(ctx, row)
		if err != nil {
			return nil, err
		}
		defer func() {
			tblIter.Close(ctx)
		}()

		// TODO: helper method probably
		count := 0
		means := make([]float64, len(resTbl.Schema()))
		mins := make([]float64, len(resTbl.Schema()))
		maxs := make([]float64, len(resTbl.Schema()))
		for i := 0; i < len(resTbl.Schema()); i++ {
			mins[i] = math.MaxFloat64
			maxs[i] = -math.MaxFloat64 // not sure if this is right
		}

		for {
			row, err := tblIter.Next(ctx)
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}

			// accumulate sum of every column
			// TODO: deal with non-numeric types, precision, and overflow
			for i := 0; i < len(resTbl.Schema()); i++ {
				num, err := sql.Float64.Convert(row[i])
				if err != nil {
					return nil, err
				}
				numFloat := num.(float64)
				means[i] += numFloat
				mins[i] = math.Min(numFloat, mins[i])
				maxs[i] = math.Max(numFloat, maxs[i])
			}

			// TODO: means, median, not null
			count++
		}

		// Go through each column of table we want to analyze
		for i, col := range resTbl.Schema() {
			// Create Primary Key for lookup
			colStatsPk := mysql_db.ColumnStatisticsPrimaryKey{
				SchemaName: database,
				TableName:  resTbl.Name(),
				ColumnName: col.Name,
			}

			// Remove if existing
			existingRows := colStatsTableData.Get(colStatsPk)
			for _, row := range existingRows {
				colStatsTableData.Remove(ctx, colStatsPk, row)
			}

			// Insert row entry
			colStatsTableData.Put(ctx, &mysql_db.ColumnStatistics{
				SchemaName: database,
				TableName:  resTbl.Name(),
				ColumnName: col.Name,
				Count:      uint64(count),
				Mean:       means[i] / float64(count),
				Min:        mins[i],
				Max:        maxs[i],
			})
		}
	}

	return sql.RowsToRowIter(sql.Row{sql.NewOkResult(0)}), nil
}
