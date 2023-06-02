// Copyright 2023 Dolthub, Inc.
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
	gmstime "github.com/dolthub/go-mysql-server/internal/time"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/types"
	"strings"
)

type ShowCreateEvent struct {
	db        sql.Database
	EventName string
	Event     sql.EventDetails
}

var _ sql.Databaser = (*ShowCreateEvent)(nil)
var _ sql.Node = (*ShowCreateEvent)(nil)
var _ sql.CollationCoercible = (*ShowCreateEvent)(nil)

var showCreateEventSchema = sql.Schema{
	&sql.Column{Name: "Event", Type: types.LongText, Nullable: false},
	&sql.Column{Name: "sql_mode", Type: types.LongText, Nullable: false},
	&sql.Column{Name: "time_zone", Type: types.LongText, Nullable: false},
	&sql.Column{Name: "Create Event", Type: types.LongText, Nullable: false},
	&sql.Column{Name: "character_set_client", Type: types.LongText, Nullable: false},
	&sql.Column{Name: "collation_connection", Type: types.LongText, Nullable: false},
	&sql.Column{Name: "Database Collation", Type: types.LongText, Nullable: false},
}

// NewShowCreateEvent creates a new ShowCreateEvent node for SHOW CREATE EVENT statements.
func NewShowCreateEvent(db sql.Database, event string) *ShowCreateEvent {
	return &ShowCreateEvent{
		db:        db,
		EventName: strings.ToLower(event),
	}
}

// String implements the sql.Node interface.
func (s *ShowCreateEvent) String() string {
	return fmt.Sprintf("SHOW CREATE EVENT %s", s.EventName)
}

// Resolved implements the sql.Node interface.
func (s *ShowCreateEvent) Resolved() bool {
	_, ok := s.db.(sql.UnresolvedDatabase)
	return !ok
}

// Children implements the sql.Node interface.
func (s *ShowCreateEvent) Children() []sql.Node {
	return nil
}

// Schema implements the sql.Node interface.
func (s *ShowCreateEvent) Schema() sql.Schema {
	return showCreateEventSchema
}

// RowIter implements the sql.Node interface.
func (s *ShowCreateEvent) RowIter(ctx *sql.Context, row sql.Row) (sql.RowIter, error) {
	characterSetClient, err := ctx.GetSessionVariable(ctx, "character_set_client")
	if err != nil {
		return nil, err
	}
	collationConnection, err := ctx.GetSessionVariable(ctx, "collation_connection")
	if err != nil {
		return nil, err
	}
	collationServer, err := ctx.GetSessionVariable(ctx, "collation_server")
	if err != nil {
		return nil, err
	}

	e := s.Event.ConvertTimesFromUTCToTz(gmstime.SystemTimezoneOffset())
	sysVal, err := ctx.Session.GetSessionVariable(ctx, "sql_mode")
	if err != nil {
		return nil, err
	}
	sqlMode, sok := sysVal.(string)
	if !sok {
		return nil, sql.ErrSystemVariableCodeFail.New("sql_mode", sysVal)
	}
	// TODO: fill time_zone with appropriate value, for now it's default to SYSTEM
	return sql.RowsToRowIter(sql.Row{
		e.Name,                   // Event
		sqlMode,                  // sql_mode
		"SYSTEM",                 // time_zone
		e.CreateEventStatement(), // Create Event
		characterSetClient,       // character_set_client
		collationConnection,      // collation_connection
		collationServer,          // Database Collation
	}), nil
}

// WithChildren implements the sql.Node interface.
func (s *ShowCreateEvent) WithChildren(children ...sql.Node) (sql.Node, error) {
	return NillaryWithChildren(s, children...)
}

// CheckPrivileges implements the interface sql.Node.
func (s *ShowCreateEvent) CheckPrivileges(ctx *sql.Context, opChecker sql.PrivilegedOperationChecker) bool {
	// TODO: figure out what privileges are needed here
	return true
}

// CollationCoercibility implements the interface sql.CollationCoercible.
func (*ShowCreateEvent) CollationCoercibility(ctx *sql.Context) (collation sql.CollationID, coercibility byte) {
	return sql.Collation_binary, 7
}

// Database implements the sql.Databaser interface.
func (s *ShowCreateEvent) Database() sql.Database {
	return s.db
}

// WithDatabase implements the sql.Databaser interface.
func (s *ShowCreateEvent) WithDatabase(db sql.Database) (sql.Node, error) {
	ns := *s
	ns.db = db
	return &ns, nil
}
