// Options for composing store queries — modelled on kodit's
// domain/repository.Option (github.com/helixml/kodit/domain/repository).
// Per-entity store interfaces keep their named methods (Get, List,
// Update, Delete) for readability at call sites, but the gorm impl
// builds every query through the Option chain so per-entity gorm code
// is a thin mapping layer, not 100+ lines of gorm.WithContext / Where
// / Order boilerplate per repo.
//
// The shape mirrors kodit:
//   - Option func(Query) Query — composable query builder
//   - Query holds conditions, raw clauses, joins, ordering, paging
//   - WithCondition / WithConditionIn / WithLimit / WithOrderAsc /
//     WithOrderDesc / WithWhere are the primitives
//   - Per-store helpers like WithOrg, WithStreamID delegate to those
//     primitives
//
// The infrastructure side (api/pkg/org/store/gorm) translates Query
// into gorm.DB calls via ApplyOptions.
package store

// Option modifies a Query. Compose with Build(opts...) or pass
// straight into a Repository method.
type Option func(Query) Query

// Query is the abstract filter / ordering / pagination state that
// options accumulate. Gorm — and any future driver — translates it
// into the dialect-specific call sequence.
type Query struct {
	conditions []Condition
	clauses    []Clause
	orders     []Order
	joins      []Join
	table      string
	selects    string
	limit      int
	updates    map[string]any
}

// Build folds opts onto an empty Query, left-to-right.
func Build(opts ...Option) Query {
	q := Query{}
	for _, opt := range opts {
		q = opt(q)
	}
	return q
}

// Conditions returns the equality / IN filters.
func (q Query) Conditions() []Condition { return append([]Condition(nil), q.conditions...) }

// Clauses returns the raw WHERE clauses.
func (q Query) Clauses() []Clause { return append([]Clause(nil), q.clauses...) }

// Orders returns the ORDER BY specs.
func (q Query) Orders() []Order { return append([]Order(nil), q.orders...) }

// Joins returns the JOIN clauses.
func (q Query) Joins() []Join { return append([]Join(nil), q.joins...) }

// Table returns the override FROM table (empty when unset).
func (q Query) Table() string { return q.table }

// Selects returns the SELECT projection (empty when unset).
func (q Query) Selects() string { return q.selects }

// Limit returns the row cap (0 means unbounded).
func (q Query) Limit() int { return q.limit }

// Updates returns the column → value map for UPDATE statements.
func (q Query) Updates() map[string]any {
	if q.updates == nil {
		return nil
	}
	out := make(map[string]any, len(q.updates))
	for k, v := range q.updates {
		out[k] = v
	}
	return out
}

// Condition is a `field = value` or `field IN values` filter.
type Condition struct {
	Field string
	Value any
	In    bool
}

// Clause is a raw WHERE expression with bind args.
type Clause struct {
	SQL  string
	Args []any
}

// Order is an ORDER BY spec.
type Order struct {
	Expr      string // either a column name or a raw expression
	Ascending bool
}

// Join is a JOIN clause — gorm-compatible raw join string with bind
// args (e.g. `JOIN other AS o ON o.x = e.y`). Used for the few queries
// (events listForWorker / listSince) where filter columns live on a
// joined table.
type Join struct {
	SQL  string
	Args []any
}

// ---- generic primitives -------------------------------------------------

// WithCondition adds `field = value`.
func WithCondition(field string, value any) Option {
	return func(q Query) Query {
		q.conditions = append(q.conditions, Condition{Field: field, Value: value})
		return q
	}
}

// WithConditionIn adds `field IN (values)`. values is typically a
// slice.
func WithConditionIn(field string, values any) Option {
	return func(q Query) Query {
		q.conditions = append(q.conditions, Condition{Field: field, Value: values, In: true})
		return q
	}
}

// WithWhere appends a raw WHERE clause with bind args. Use when the
// column-name helpers can't express it (OR, IS NULL, subqueries).
func WithWhere(sql string, args ...any) Option {
	return func(q Query) Query {
		q.clauses = append(q.clauses, Clause{SQL: sql, Args: args})
		return q
	}
}

// WithOrderAsc appends ORDER BY <expr> ASC.
func WithOrderAsc(expr string) Option {
	return func(q Query) Query {
		q.orders = append(q.orders, Order{Expr: expr, Ascending: true})
		return q
	}
}

// WithOrderDesc appends ORDER BY <expr> DESC.
func WithOrderDesc(expr string) Option {
	return func(q Query) Query {
		q.orders = append(q.orders, Order{Expr: expr, Ascending: false})
		return q
	}
}

// WithLimit caps the row count. limit <= 0 is a no-op so callers can
// pass through user-supplied limits unchanged.
func WithLimit(limit int) Option {
	return func(q Query) Query {
		if limit > 0 {
			q.limit = limit
		}
		return q
	}
}

// WithJoin appends a JOIN clause. Use when the filter / projection
// needs columns from another table (events ListForWorker, ListSince).
func WithJoin(sql string, args ...any) Option {
	return func(q Query) Query {
		q.joins = append(q.joins, Join{SQL: sql, Args: args})
		return q
	}
}

// WithTable overrides the FROM table. Required when the Repository's
// gorm.Model(R) default would clash with a JOIN that aliases the row
// table (e.g. `org_events AS e`).
func WithTable(name string) Option {
	return func(q Query) Query {
		q.table = name
		return q
	}
}

// WithSelect sets the SELECT projection. Mainly useful with JOINs so
// the row scanner sees only the entity table's columns.
func WithSelect(projection string) Option {
	return func(q Query) Query {
		q.selects = projection
		return q
	}
}

// WithUpdates sets the column-value map for UPDATE statements.
// Multiple calls merge.
func WithUpdates(set map[string]any) Option {
	return func(q Query) Query {
		if q.updates == nil {
			q.updates = make(map[string]any, len(set))
		}
		for k, v := range set {
			q.updates[k] = v
		}
		return q
	}
}

// ---- org-graph shared options ------------------------------------------

// WithOrg constrains to a single helix tenant. Composite (id,
// org_id) PKs mean every per-entity Get / List / Update / Delete
// reaches for this.
func WithOrg(orgID string) Option { return WithCondition("org_id", orgID) }

// WithID matches the entity's `id` column. Wraps string-like IDs
// (`orgchart.RoleID`, `orgchart.WorkerID`, …) via Stringer-style conversion at the
// call site.
func WithID(id any) Option { return WithCondition("id", id) }
