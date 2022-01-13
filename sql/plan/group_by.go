// Copyright 2020-2021 Dolthub, Inc.
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

package plan

import (
	"fmt"
	"strings"

	opentracing "github.com/opentracing/opentracing-go"
	errors "gopkg.in/src-d/go-errors.v1"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/expression"
	"github.com/dolthub/go-mysql-server/sql/expression/function/aggregation"
)

// ErrGroupBy is returned when the aggregation is not supported.
var ErrGroupBy = errors.NewKind("group by aggregation '%v' not supported")

// GroupBy groups the rows by some expressions.
type GroupBy struct {
	UnaryNode
	SelectedExprs []sql.Expression
	GroupByExprs  []sql.Expression
}

// NewGroupBy creates a new GroupBy node. Like Project, GroupBy is a top-level node, and contains all the fields that
// will appear in the output of the query. Some of these fields may be aggregate functions, some may be columns or
// other expressions. Unlike a project, the GroupBy also has a list of group-by expressions, which usually also appear
// in the list of selected expressions.
func NewGroupBy(selectedExprs, groupByExprs []sql.Expression, child sql.Node) *GroupBy {
	return &GroupBy{
		UnaryNode:     UnaryNode{Child: child},
		SelectedExprs: selectedExprs,
		GroupByExprs:  groupByExprs,
	}
}

// Resolved implements the Resolvable interface.
func (g *GroupBy) Resolved() bool {
	return g.UnaryNode.Child.Resolved() &&
		expression.ExpressionsResolved(g.SelectedExprs...) &&
		expression.ExpressionsResolved(g.GroupByExprs...)
}

// Schema implements the Node interface.
func (g *GroupBy) Schema() sql.Schema {
	var s = make(sql.Schema, len(g.SelectedExprs))
	for i, e := range g.SelectedExprs {
		var name string
		if n, ok := e.(sql.Nameable); ok {
			name = n.Name()
		} else {
			name = e.String()
		}

		var table string
		if t, ok := e.(sql.Tableable); ok {
			table = t.Table()
		}

		s[i] = &sql.Column{
			Name:     name,
			Type:     e.Type(),
			Nullable: e.IsNullable(),
			Source:   table,
		}
	}

	return s
}

// RowIter implements the Node interface.
func (g *GroupBy) RowIter(ctx *sql.Context, row sql.Row) (sql.RowIter, error) {
	span, ctx := ctx.Span("plan.GroupBy", opentracing.Tags{
		"groupings":  len(g.GroupByExprs),
		"aggregates": len(g.SelectedExprs),
	})

	i, err := g.Child.RowIter(ctx, row)
	if err != nil {
		span.Finish()
		return nil, err
	}

	aggs := make([]*aggregation.Aggregation, len(g.SelectedExprs))
	for i, e := range g.SelectedExprs {
		switch a := e.(type) {
		case sql.WindowAdaptableExpression:
			fn, err := a.NewWindowFunction()
			if err != nil {
				return nil, err
			}
			aggs[i] = aggregation.NewAggregation(fn, aggregation.NewGroupByFramer())
		default:
			fn, err := aggregation.NewLast(a).NewWindowFunction()
			if err != nil {
				return nil, err
			}
			aggs[i] = aggregation.NewAggregation(fn, aggregation.NewGroupByFramer())
		}
	}
	iter := aggregation.NewWindowBlockIter(g.GroupByExprs, nil, aggs, i)

	return sql.NewSpanIter(span, iter), nil
}

// WithChildren implements the Node interface.
func (g *GroupBy) WithChildren(children ...sql.Node) (sql.Node, error) {
	if len(children) != 1 {
		return nil, sql.ErrInvalidChildrenNumber.New(g, len(children), 1)
	}

	return NewGroupBy(g.SelectedExprs, g.GroupByExprs, children[0]), nil
}

// WithExpressions implements the Node interface.
func (g *GroupBy) WithExpressions(exprs ...sql.Expression) (sql.Node, error) {
	expected := len(g.SelectedExprs) + len(g.GroupByExprs)
	if len(exprs) != expected {
		return nil, sql.ErrInvalidChildrenNumber.New(g, len(exprs), expected)
	}

	agg := make([]sql.Expression, len(g.SelectedExprs))
	copy(agg, exprs[:len(g.SelectedExprs)])

	grouping := make([]sql.Expression, len(g.GroupByExprs))
	copy(grouping, exprs[len(g.SelectedExprs):])

	return NewGroupBy(agg, grouping, g.Child), nil
}

func (g *GroupBy) String() string {
	pr := sql.NewTreePrinter()
	_ = pr.WriteNode("GroupBy")

	var selectedExprs = make([]string, len(g.SelectedExprs))
	for i, e := range g.SelectedExprs {
		selectedExprs[i] = e.String()
	}

	var grouping = make([]string, len(g.GroupByExprs))
	for i, g := range g.GroupByExprs {
		grouping[i] = g.String()
	}

	_ = pr.WriteChildren(
		fmt.Sprintf("SelectedExprs(%s)", strings.Join(selectedExprs, ", ")),
		fmt.Sprintf("Grouping(%s)", strings.Join(grouping, ", ")),
		g.Child.String(),
	)
	return pr.String()
}

func (g *GroupBy) DebugString() string {
	pr := sql.NewTreePrinter()
	_ = pr.WriteNode("GroupBy")

	var selectedExprs = make([]string, len(g.SelectedExprs))
	for i, e := range g.SelectedExprs {
		selectedExprs[i] = sql.DebugString(e)
	}

	var grouping = make([]string, len(g.GroupByExprs))
	for i, g := range g.GroupByExprs {
		grouping[i] = sql.DebugString(g)
	}

	_ = pr.WriteChildren(
		fmt.Sprintf("SelectedExprs(%s)", strings.Join(selectedExprs, ", ")),
		fmt.Sprintf("Grouping(%s)", strings.Join(grouping, ", ")),
		sql.DebugString(g.Child),
	)
	return pr.String()
}

// Expressions implements the Expressioner interface.
func (g *GroupBy) Expressions() []sql.Expression {
	var exprs []sql.Expression
	exprs = append(exprs, g.SelectedExprs...)
	exprs = append(exprs, g.GroupByExprs...)
	return exprs
}
