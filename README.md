# dbutil

Generic utilities for building nested data structures from relational queries without the N+1 problem.

- **Minimal** — 3 generic functions, zero library dependencies.
- **Composable** — works with sqlx, sqlc, pgx, or any query tool. No framework lock-in.
- **Solves the real problem** — eliminates N+1 without a full ORM or monolithic JOINs.
- **Any depth** — the bottom-up pattern avoids nested loops regardless of nesting level.

## The Problem

When building nested responses (e.g. `Category → Products → Variants`), common approaches are:

- **Single JOIN**: returns flat rows that are hard to map back into nested Go structs, and falls apart when each level needs its own filters.
- **N+1 queries**: one query for parents, then one per parent for children. Simple but doesn't scale.

## How Other Frameworks Solve This

This idea is directly inspired by how major ORMs handle eager loading:

| Framework | Feature |
|-----------|---------|
| **Laravel Eloquent** (PHP) | `with()` / `load()` — queries each level with `WHERE IN`, reconstructs in memory |
| **Hibernate** (Java) | `@BatchSize`, `JOIN FETCH` — configurable batch fetching per relationship |

All of them internally do the same thing: query parents, collect IDs, query children with `WHERE parent_id IN (...)`, and reconstruct the graph in memory. This library exposes those three steps as simple Go generics — no ORM required.

## Is This Library Useful?

Yes, if you:

- Use **sqlx**, **sqlc**, **pgx**, or any query builder — but still need to build nested structs without N+1.
- Don't want a full ORM but want its eager-loading pattern.
- Need **per-level filtering/pagination** (impossible with a single JOIN).
- Want something minimal: 3 functions, zero library dependencies.

### When to Use This Library Over Database-Side JSON

Many modern databases can build nested JSON directly (e.g. `JSON_AGG`, `JSON_BUILD_OBJECT` in PostgreSQL). However, this library is a better fit when:

- **Your DB doesn't support JSON functions** — SQLite, MySQL < 5.7, older PostgreSQL versions, or any legacy/commercial DB without JSON aggregation.
- **You need per-level filtering** — database JSON aggregation forces a single query; you cannot apply different WHERE/pagination/sorting per nesting level. With this library each level is an independent query.
- **You want type safety** — JSON aggregation returns `[]byte` or a driver-specific type you must deserialize. This library works directly with typed Go structs via generics.
- **The reconstruction is complex** — deeply nested structures with 4+ levels become unwieldy in SQL but remain flat and readable in Go.
- **You avoid vendor lock-in** — the same Go code works with PostgreSQL, MySQL, SQLite, SQL Server, etc. without changing a single line.

### What Makes This Library Stand Out

| Concern | This library | JSON in SQL | ORM |
|---------|-------------|-------------|-----|
| Per-level filters | ✅ Yes | ❌ Single query | ✅ Yes |
| DB-agnostic | ✅ Any driver | ❌ PostgreSQL only | ✅ Most ORMs |
| No runtime deps | ✅ 0 dependencies | ✅ None | ❌ Heavy |
| Type-safe | ✅ Generics | ❌ `[]byte` | ✅ Usually |
| Learning curve | ~3 functions | ❌ Complex SQL | ❌ Large API surface |
| N+1 elimination | ✅ Batch WHERE IN | ✅ Single query | ✅ Eager loading |

It won't replace an ORM for every use case, but for the common pattern of "query parents → batch-query children → merge", it removes the boilerplate.

## The Pattern

Query each level independently with `WHERE IN`, then use these utilities to reconstruct the hierarchy.

```
SELECT * FROM categories              → []Parent
ExtractIDs(parents)                   → []id
SELECT * FROM children WHERE parent_id IN (…) → []Child
GroupBy(children, parent_id)          → map[id][]Child
MergeChildren(parents, grouped)       → parents now have their children
```

Each level can have its own filters, pagination, or business logic.

## Functions

```go
// Groups a slice into a map keyed by K.
func GroupBy[T any, K comparable](objects []T, keyFunc func(T) K) map[K][]T

// Extracts a key from each element, preserving order.
func ExtractIDs[T any, KEY any](objects []T, getKeyFunc func(T) KEY) []KEY

// Merges grouped children into their parents in-place.
func MergeChildren[TParent, TChild any, K comparable](
    parents []TParent,
    childMap map[K][]TChild,
    parentKeyFunc func(TParent) K,
    setChildrenFunc func(*TParent, []TChild),
)
```

## Examples

### With sqlx

```go
import "github.com/jmoiron/sqlx"

// 1. Parents
var cats []Category
db.Select(&cats, "SELECT id, name FROM categories")

// 2. Children (batch)
ids := ExtractIDs(cats, func(c Category) int { return c.ID })
q, args, _ := sqlx.In("SELECT id, category_id, name FROM products WHERE category_id IN (?)", ids)
var prods []Product
db.Select(&prods, q, args...)

// 3. Group & merge
byCat := GroupBy(prods, func(p Product) int { return p.CategoryID })
MergeChildren(cats, byCat,
    func(c Category) int { return c.ID },
    func(c *Category, ps []Product) { c.Products = ps },
)

// 4. Query grandchildren (top-down)
prodIDs := ExtractIDs(prods, func(p Product) int { return p.ID })
q, args, _ = sqlx.In("SELECT id, product_id, name, price FROM variants WHERE product_id IN (?)", prodIDs)
var vars []Variant
db.Select(&vars, q, args...)

// 5. Reconstruct bottom-up (flat slices, no nested loops)
byProd := GroupBy(vars, func(v Variant) int { return v.ProductID })
MergeChildren(prods, byProd,
    func(p Product) int { return p.ID },
    func(p *Product, vs []Variant) { p.Variants = vs },
)

byCat := GroupBy(prods, func(p Product) int { return p.CategoryID })
MergeChildren(cats, byCat,
    func(c Category) int { return c.ID },
    func(c *Category, ps []Product) { c.Products = ps },
)
```

Output:

```json
[
  {
    "id": 1,
    "name": "Alice",
    "posts": [
      {
        "id": 1,
        "author_id": 1,
        "title": "First Post",
        "comments": [
          { "id": 1, "post_id": 1, "author": "Charlie", "body": "Great post!" },
          { "id": 2, "post_id": 1, "author": "Diana", "body": "Thanks!" }
        ]
      },
      {
        "id": 2,
        "author_id": 1,
        "title": "Second Post",
        "comments": [
          { "id": 3, "post_id": 2, "author": "Eve", "body": "Nice write-up" }
        ]
      }
    ]
  },
  {
    "id": 2,
    "name": "Bob",
    "posts": [
      {
        "id": 3,
        "author_id": 2,
        "title": "Hello World",
        "comments": [
          { "id": 4, "post_id": 3, "author": "Frank", "body": "First comment!" },
          { "id": 5, "post_id": 3, "author": "Grace", "body": "Awesome blog" }
        ]
      }
    ]
  }
]
```

### With sqlc

sqlc generates type-safe query functions. After calling them, use the same utilities:

```go
// 1. Parents (sqlc-generated)
cats, _ := queries.ListCategories(ctx, db)

// 2. Children batch (sqlc-generated)
ids := dbutil.ExtractIDs(cats, func(c Category) int32 { return c.ID })
prods, _ := queries.ListProductsByCategoryIDs(ctx, db, ids)

// 3. Group & merge
byCat := dbutil.GroupBy(prods, func(p Product) int32 { return p.CategoryID })
dbutil.MergeChildren(cats, byCat,
    func(c Category) int32 { return c.ID },
    func(c *Category, ps []Product) { c.Products = ps },
)

// 4. Query grandchildren (top-down)
prodIDs := dbutil.ExtractIDs(prods, func(p Product) int32 { return p.ID })
vars, _ := queries.ListVariantsByProductIDs(ctx, db, prodIDs)

// 5. Reconstruct bottom-up (flat slices, no nested loops)
byProd := dbutil.GroupBy(vars, func(v Variant) int32 { return v.ProductID })
dbutil.MergeChildren(prods, byProd,
    func(p Product) int32 { return p.ID },
    func(p *Product, vs []Variant) { p.Variants = vs },
)

byCat = dbutil.GroupBy(prods, func(p Product) int32 { return p.CategoryID })
dbutil.MergeChildren(cats, byCat,
    func(c Category) int32 { return c.ID },
    func(c *Category, ps []Product) { c.Products = ps },
)
```

Output (with per-level filters: only active categories, published products with price > 0, variants with stock > 0, inventory with quantity > 0):

```json
[
  {
    "id": 1,
    "name": "Clothing",
    "active": true,
    "products": [
      {
        "id": 1,
        "category_id": 1,
        "name": "T-Shirt",
        "price": 19.99,
        "published": true,
        "variants": [
          {
            "id": 1,
            "product_id": 1,
            "name": "Small",
            "stock": 10,
            "inventory": [
              { "id": 1, "variant_id": 1, "warehouse": "Warehouse A", "quantity": 20 },
              { "id": 2, "variant_id": 1, "warehouse": "Warehouse B", "quantity": 5 }
            ]
          },
          {
            "id": 3,
            "product_id": 1,
            "name": "Large",
            "stock": 5,
            "inventory": [
              { "id": 3, "variant_id": 3, "warehouse": "Warehouse A", "quantity": 10 }
            ]
          }
        ]
      }
    ]
  },
  {
    "id": 2,
    "name": "Electronics",
    "active": true,
    "products": [
      {
        "id": 3,
        "category_id": 2,
        "name": "Headphones",
        "price": 99.99,
        "published": true,
        "variants": [
          {
            "id": 4,
            "product_id": 3,
            "name": "Wired",
            "stock": 3,
            "inventory": [
              { "id": 5, "variant_id": 4, "warehouse": "Warehouse B", "quantity": 8 }
            ]
          }
        ]
      }
    ]
  }
]
```

## Deeper Nesting

For 3+ levels, use bottom-up reconstruction to avoid nested loops.

**Phase 1**: Query all levels top-down with `WHERE IN`.

**Phase 2**: Reconstruct bottom-up — each `MergeChildren` operates on a flat slice:

```go
// Query phase (top-down)
continents := queryContinents(db)
countries := queryCountriesByIDs(db, ExtractIDs(continents, ...))
cities    := queryCitiesByIDs(db, ExtractIDs(countries, ...))
districts := queryDistrictsByIDs(db, ExtractIDs(cities, ...))

// Reconstruct phase (bottom-up, flat slices only)
MergeChildren(cities,    GroupBy(districts, districtCityID), ...)  // flat
MergeChildren(countries, GroupBy(cities,    cityCountryID), ...)  // flat
MergeChildren(continents,GroupBy(countries, countryContID), ...)  // flat
```

No nested `for` loops regardless of depth. Any depth N requires exactly N queries and N-1 flat `MergeChildren` calls.

See `dbutil_test.go` for full 3-level, 4-level, and 6-level examples.
