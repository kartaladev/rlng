# rlng

A **pure-Go rule engine library**, built on top of [expr-lang/expr](https://github.com/expr-lang/expr).

`rlng` compiles declarative rule / expression definitions (YAML or JSON) and evaluates
them against runtime input. It is the *engine* only — not a Business Rules Management
System: no authoring UI, governance, or persistence, just fast, embeddable evaluation.

> **Status: unreleased (pre-`v0.0.1`).** All five build increments are implemented and
> merged — the `expr`, `pipe`, and `config` packages and the root `rlng` facade are
> complete, tested (`-race`), and lint-clean. The exported API is stable but **not yet
> tagged**; pin to a commit until `v0.0.1` is cut. APIs may still change before the first
> tag. See [`CLAUDE.md`](./CLAUDE.md) for the architecture and contributor workflow.

## Why another rule engine?

A rule that decides money, credit, or eligibility is only as good as your ability to trust,
trace, and debug it. `rlng` is built around three properties that make that possible:

- **Pure Go, no cgo** — the evaluator is ordinary Go you can set a breakpoint in, step
  through line by line, and read a plain Go stack trace from. There is no foreign VM to
  treat as a black box, and the whole library stays trivially cross-compilable
  (`CGO_ENABLED=0 go build`).
- **Explainable** — a decision can trace itself back to the exact rules and seed inputs that
  produced it: a recorded firing trail per stage, value **provenance/lineage**, and an
  `Explain` that prints the derivation of any output down to the original inputs.
- **Debuggable** — failures surface as typed, `Unwrap`-able errors that name the offending
  field and expression, so a bad rule points at *itself* rather than dying in an opaque
  evaluator.

## Design at a glance

Rules are declared as config and compiled once, then evaluated on the hot path:

- **Flexible config — declarative *and* programmatic** — the *same* engine can be built two
  ways. Author it as a **YAML/JSON document** (loaded via pluggable sources, with
  pipeline-level `constants`, an output `mapping` block, and an opt-in `schema` block for
  strict type-checking) so a whole decision service is one file; or wire it **entirely in Go**
  by constructing stages directly (`pipe.NewSingleExpr` / `pipe.NewDecisionTable` /
  `pipe.NewPipeline`). The two paths are interchangeable and converge on the same
  `*pipe.Pipeline` — config just parses-and-builds what you could have assembled by hand.
  Strict mode (`WithStrict()` / `WithSchema()`) and lint-as-error (`WithLintErrors()`, for
  missing defaults and unreachable rules) apply on the declarative path.
- **Staged evaluation** — stages (single-expression, multi-expression, and decision-table)
  are ordered by their declared dependencies (a topologically sorted DAG).
- **Decision tables** — ordered `condition → decisions` rules with hit policies
  **single / unique / any / collect**, a per-table **default (else)** branch, and collect
  **aggregation** (`sum`/`min`/`max`/`count`/list).
- **Exact-decimal money & value fidelity** — an in-expression `decimal` type (built on
  [shopspring/decimal](https://github.com/shopspring/decimal)) with arithmetic operator
  overloads and `round`/`roundBank` (banker's) rounding, so `$250,000 × 7.25%` is exactly
  `$18,125.00`, not the `18124.999…` a `float64` produces. A value keeps its **type and
  precision at every serde boundary**: declarable as a config constant (`{"$dec": "0.0725"}`),
  preserved through the struct/map seed, numerically-exact in aggregation (integer sums stay
  `int64` with checked overflow; no float round-trip), and — via a **canonically type-tagged
  Scope JSON** (`"v":2`, backward-compatible reads) — reloaded as the *same kind* so a
  persisted decision replays losslessly.
- **Explainable decisions** — optional rule `id`/`message`, a recorded firing trail per
  stage (`FiringRule` for the first/only match, `FiringRulesFor` for every rule a
  **collect**/**any** table matched), value **provenance/lineage**, and **per-stage timing**.
- **Ruleset identity & replay** — a deterministic content `Hash()` plus an author
  `version` label stamped onto every `Scope` (`Scope.Ruleset()`), and a `MatchesRuleset`
  replay-safety check, so a persisted decision round-trips as a self-describing,
  replayable record (which ruleset produced it, and whether a candidate ruleset matches).
- **Strict typed evaluation** — opt-in `expr.WithEnv` rejects field typos and type errors
  at compile time instead of silently evaluating to nil.
- **Extensible** — register host functions (`expr.WithFunction`), including a clock-backed
  `now()` for deterministic temporal rules.
- **Ruleset lint** — static checks for unreachable rules and missing-default coverage gaps.
- **Typed result mapping** — the evaluated context is projected into a caller-supplied Go
  type via a mapping template.

See [`CLAUDE.md`](./CLAUDE.md) for the full architecture blueprint and contributor workflow,
and [`examples/`](./examples) for runnable end-to-end examples.

## Usage

`rlng` gives you a typed facade (the root `rlng` package) sitting on three building blocks you
can also reach for directly: `expr` (compile one expression), `pipe` (wire expressions into
stages and pipelines), and `config` (author the whole thing as YAML/JSON). This walkthrough
climbs the same ladder the runnable [`examples/`](./examples) tour does — one real problem at a
time, simplest first — and for each concept calls out the **quirks** worth knowing and a
**table** of the options you can reach for. The snippets are illustrative distillations, not
verbatim copies; run `go test ./examples/ -v` for the real, working code.

### 1. One-off expressions — a predicate and a value

You have a single business question — *is this applicant an adult and verified?*, *what's
`price × qty`?* — and reaching for a whole pipeline would be overkill. Compile the expression
once and evaluate it against any environment map as often as you like:

```go
gate, _ := expr.NewPredicate("age >= 18 && verified")
ok, _ := gate.Test(map[string]any{"age": 34, "verified": true}) // true

total, _ := expr.NewFunction("total", "price * qty")
v, _ := total.Apply(map[string]any{"price": 10, "qty": 3}) // 30
```

**Watch out:**

- A `Predicate` whose expression yields a *non-bool* fails loudly — a `*expr.EvalError`
  wrapping `expr.ErrNotBool` — rather than guessing. Opt into lenient truthiness with
  `expr.WithCoerce()`, and then the coercion has its own edge cases: `NaN`/`±Inf` and the empty
  string are false, and a `json.Number` is judged by its string form.
- A `Function` returns whatever the expression evaluates to; `nil` is a legitimate result, not
  an error (see `WithFallbackOnNil` if you disagree).

| Option | What it does | Quirk / default |
| --- | --- | --- |
| `WithCoerce()` | `Predicate` only: lenient truthiness instead of bool-only | Default is strict (bool-only). `NaN`/`±Inf`/`""` are false |
| `WithFallback(expr)` | `Function` only: expression to evaluate when the main one **errors** | Compiled eagerly; evaluated over an **empty** env (no access to the input) |
| `WithFallbackOnNil()` | Also fire the fallback when the main expression yields `nil` | Default: `nil` is a first-class result, fallback fires only on error |
| `WithFallbackObserver(fn)` | `func(name, expression string, cause error)` observing an **error**-triggered fallback | Not called for a `nil`-triggered fallback — the masked error is otherwise silent |
| `WithReturnKind(k)` | `Function` only: coerce the result (and fallback) to a `reflect.Kind` | Applies to both main and fallback results |

See [`examples/01_expr_predicates_test.go`](./examples/01_expr_predicates_test.go) and
[`examples/02_expr_functions_test.go`](./examples/02_expr_functions_test.go).

### 2. Variable defaults & strict typing

Real rules have thresholds — a minimum score, a cap — that you want to declare *once* as a
default and still let a caller override at runtime. And once a ruleset has a few dozen field
names, a typo like `scoer` silently evaluating to `nil` is exactly the bug you don't want.
`WithGlobals`/`WithLocals` compile in `x ?? <default>` fallbacks (overridable by input);
`WithEnv` turns on strict type-checking so the typo fails at *compile* time:

```go
gate, _ := expr.NewPredicate("score >= minScore",
	expr.WithGlobals(map[string]any{"minScore": 650}))
ok, _ := gate.Test(map[string]any{"score": 680}) // true — no override needed

_, err := expr.NewPredicate("scoer >= 650", expr.WithEnv(map[string]any{"score": 0}))
// err is a *expr.CompileError: "scoer" isn't in the declared env
```

**Watch out:**

- **Locals win over globals**, and repeated `WithGlobals`/`WithLocals` calls *merge* (last
  value wins per key) rather than the later call discarding earlier keys.
- Without `WithEnv`, an undefined name is tolerated as `nil` forever — convenient for pipelines
  (a stage can reference a not-yet-computed path), dangerous for a hand-written rule.
- `WithEnv` values are *representative* — you pass a value of the right type (e.g. `0` for an
  int field), not the real datum.

| Option | What it does | Quirk / default |
| --- | --- | --- |
| `WithGlobals(map)` | Engine-wide defaults, injected as `x ?? <default>` | Merges across calls (last wins); overridden by locals and by runtime input |
| `WithLocals(map)` | Per-evaluator defaults, same `??` mechanism | **Take precedence over globals**; also merge across calls |
| `WithEnv(map)` | Strict compile against a declared type environment | Drops undefined-variable tolerance; a typo becomes a `*expr.CompileError`. Globals/locals/functions are folded into the type-check env |
| `WithFunction(name, fn)` | Register a host function `func(...any) (any, error)` callable from the expression | Visible to compiler **and** VM (type-checks under `WithEnv`); registering a name twice keeps the **last** |

See [`examples/03_expr_variables_env_test.go`](./examples/03_expr_variables_env_test.go).

### 3. Exact-decimal money

You're pricing a loan and the fee has to be exact to the cent — `$250,000 × 7.25%` is
`$18,125.00`, and a `float64` that lands on `18124.999999999996` is a bug an auditor will find.
Every compiled expression has a `decimal(x)` constructor, decimal-aware arithmetic operators,
and `round`/`roundBank` builtins available with no extra wiring:

```go
fee, _ := expr.NewFunction("fee", "roundBank(decimal(principal) * decimal(rate), 2)")
v, _ := fee.Apply(map[string]any{"principal": 250_000, "rate": "0.0725"})
fmt.Println(v) // 18125.00 — not 18124.999999999996
```

**Watch out:**

- Decimal operators are resolved at **compile time** from the operands' types. A `decimal(...)`
  *wrapped* operand is exact in any mode; a **bare** `decimal.Decimal` variable (e.g.
  `principal * rate` with no wrapper) needs `expr.WithEnv` so the compiler can *see* it's a
  decimal and pick the decimal operator instead of float arithmetic.
- `round` is half-away-from-zero; `roundBank` is banker's rounding (half-to-even) — pick the
  one your jurisdiction mandates.

| Builtin | What it does | Quirk |
| --- | --- | --- |
| `decimal(x)` | Construct an exact decimal from an int, float, or numeric string | A string like `"0.0725"` is exact; a `float64` literal carries its float imprecision in — prefer strings/ints for constants |
| `round(x, n)` | Round to `n` places, **half away from zero** | `2.5 → 3`, `-2.5 → -3` |
| `roundBank(x, n)` | Round to `n` places, **banker's (half to even)** | `2.5 → 2`, `3.5 → 4` — the money default |

See [`examples/04_expr_decimal_test.go`](./examples/04_expr_decimal_test.go).

### 4. Scope & typed getters

Once you have more than one rule, you need somewhere to accumulate results. A `Scope` is the
concurrency-safe `map[string]any` threaded through a pipeline: stages write to it by
dot-separated path (`pricing.discountRate`), and typed getters read back out in **strict** or
**coercing** flavors:

```go
sc := pipe.NewScope(map[string]any{"tier": "gold"})
_ = sc.Set("pricing.discountRate", 0.10)

rate, _ := sc.GetFloat64("pricing.discountRate") // 0.1, strict
n, _ := sc.GetIntCoerce("pricing.units")         // parses "12" or 12.0 → 12
```

**Watch out:**

- The coercing getters (`…Coerce`) widen types but **never silently truncate or wrap**: a
  non-integral or overflowing value is a `*pipe.ScopeTypeError`, not a rounded-off answer.
- A missing path is `pipe.ErrPathNotFound` from every getter — distinguishable from a
  type mismatch via `errors.Is`.
- `pipe.WithStrict()` turns a second write to an already-set leaf into `pipe.ErrPathConflict`,
  which is how you catch two stages fighting over one output path.

| Getter | Reads | Quirk |
| --- | --- | --- |
| `GetInt` / `GetInt64` | Stored integer (or `int64`/`json.Number` from a JSON round-trip) | Strict: a float or string is a `*ScopeTypeError` |
| `GetFloat64` / `GetString` / `GetBool` | The value, asserted to that exact Go type | No coercion |
| `GetSlice` / `GetMap` | `[]any` / `map[string]any` | No coercion |
| `GetIntCoerce` / `GetInt64Coerce` | Any int kind, integral finite float, integer `json.Number`, base-10 string | Overflow-checked; **never** truncates — non-integral ⇒ `*ScopeTypeError` |
| `GetFloat64Coerce` | Any int/float kind, `json.Number`, numeric string | Won't manufacture `NaN`/`Inf` from text |

| Scope option | What it does | Quirk / default |
| --- | --- | --- |
| `WithStrict()` | A repeat write to a set leaf is `ErrPathConflict` | Default: last write wins silently |
| `WithProvenance()` | Record a `Derivation` per seed input and stage write | Enables `Explain`/`Lineage`/`Derivations` (see §9); small overhead |
| `WithClock(c)` | Inject a `Clock` (`clockwork`-compatible) for timing/`now()` | Defaults to a real clock; use to make tests deterministic |

See [`examples/05_pipe_scope_getters_test.go`](./examples/05_pipe_scope_getters_test.go).

### 5. Stages — single- and multi-expression

A **stage** is the unit a pipeline schedules. The two simplest: `SingleExpr` computes one value
(optionally gated by a condition) and writes it to a path; `MultiExpr` evaluates several named
expressions in **priority order** so a later one can build on an earlier one within the same
stage. Here a loyalty discount only applies to gold-tier customers:

```go
loyalty, _ := pipe.NewSingleExpr("loyaltyDiscount", "0.10",
	pipe.WithCondition(`tier == "gold"`),
	pipe.WithOutput("pricing.discountRate"))

breakdown, _ := pipe.NewMultiExpr("breakdown", []pipe.NamedExpr{
	{Name: "net", Expression: "price - price * pricing.discountRate", Priority: 1},
	{Name: "tax", Expression: "net * 0.11", Priority: 2}, // sees "net" from priority 1
})

pipeline, _ := pipe.NewPipeline([]pipe.Stage{loyalty, breakdown})
sc := pipe.NewScope(map[string]any{"tier": "gold", "price": 100.0})
_ = pipeline.Run(context.Background(), sc)
rate, _ := sc.GetFloat64("pricing.discountRate") // 0.1
```

**Watch out:**

- A **false `WithCondition` is a no-op** — the output path is left *absent*, not written as
  `false` or `0`. Downstream code should treat "missing" as "condition didn't fire".
- A `SingleExpr` with no `WithOutput` writes to a path equal to its **stage name**.
- `MultiExpr` orders by ascending `Priority` (stable for ties), so intra-stage references
  resolve top-down — but cross-*stage* ordering is the pipeline's job (§8), not priority's.

| Option | What it does | Quirk / default |
| --- | --- | --- |
| `WithCondition(expr, …)` | Gate a `SingleExpr` on a boolean predicate | False ⇒ writes **nothing**. Ignored by other stage types |
| `WithOutput(path)` | Scope path a `SingleExpr` writes to | Default: the **stage name** |
| `WithDependsOn(…)` | Declare cross-stage dependencies for DAG ordering | All stage types; see §8 |
| `WithExprOptions(…expr.Option)` | Pass `expr` options (globals, fallback, env) to a `SingleExpr`'s value expression | `SingleExpr` only |

See [`examples/06_pipe_stages_test.go`](./examples/06_pipe_stages_test.go).

### 6. Decision tables

When "the rule" is really a **table** — a stack of `condition → outputs` rows, first (or best)
match wins — a decision table says it directly. Each `Rule` has a condition and a set of named
`Decision` outputs; `WithDefault` makes "no row matched" an explicit outcome rather than a
silent gap; and the **hit policy** decides what happens when more than one row matches:

```go
grade, _ := pipe.NewDecisionTable("grade", []pipe.Rule{
	{ID: "PRIME", Condition: "score >= 750", Decisions: map[string]pipe.Decision{
		"tier":  {Expr: `"prime"`},
		"limit": {Expr: "score * 100"},
	}},
}, pipe.WithDefault(map[string]pipe.Decision{
	"tier":  {Expr: `"declined"`},
	"limit": {Expr: "0"},
}))

p, _ := pipe.NewPipeline([]pipe.Stage{grade})
sc := pipe.NewScope(map[string]any{"score": 780})
_ = p.Run(context.Background(), sc)
tier, _ := sc.GetString("grade.tier") // "prime"
fr, _ := sc.FiringRule("grade")       // fr.RuleID == "PRIME"
```

**Watch out:**

- `HitPolicyUnique`/`HitPolicyAny` exist so overlapping rows **don't** silently let the first
  one win — a genuine overlap surfaces as `ErrMultipleMatches` / `ErrConflictingMatches`.
- Under `HitPolicyCollect`, `AggregateSum` promotes to the **widest kind present**
  (`int64` → `float64` → `decimal`, integer sums overflow-checked into `ErrAggregateOverflow`),
  while `Min`/`Max` return the **original matched element** unchanged.
- A rule with an empty `Decisions` map, or an empty output key, is rejected at construction.

| Option | What it does | Quirk / default |
| --- | --- | --- |
| `WithHitPolicy(p)` | Choose how matches resolve | Default `HitPolicySingle` |
| `WithCollectAggregation(a)` | How `HitPolicyCollect` reduces each key | Default `AggregateList`; ignored unless policy is Collect |
| `WithDefault(map[string]Decision)` | Else-branch decisions when no rule matches | Recorded as a firing (default) rule |
| `WithDependsOn(…)` | Cross-stage dependencies | See §8 |

| Hit policy | Behavior | On multiple matches |
| --- | --- | --- |
| `HitPolicySingle` | First matching row wins, stop | (never conflicts — default) |
| `HitPolicyUnique` | Expect at most one match | `ErrMultipleMatches` |
| `HitPolicyAny` | Several may match, but must **agree** on shared keys | Disagreement ⇒ `ErrConflictingMatches` |
| `HitPolicyCollect` | Accumulate every match, then reduce | (that's the point — see aggregation) |

| Aggregation | Reduces the matched values to | Quirk |
| --- | --- | --- |
| `AggregateList` | A `[]any` in rule order | Default |
| `AggregateSum` | Their sum | Widest kind `int64`→`float64`→`decimal`; overflow ⇒ `ErrAggregateOverflow`; non-numeric ⇒ `ErrNonNumericAggregate` |
| `AggregateMin` / `AggregateMax` | The smallest / largest | Returns the **original** matched element (its type preserved) |
| `AggregateCount` | The number of matches | Always an `int` |

See [`examples/07_pipe_decision_table_test.go`](./examples/07_pipe_decision_table_test.go).

### 7. `foreach` line-items

Some decisions run per line — every piece of collateral, every invoice line, every basket item.
A `foreach` stage runs an inner pipeline once per collection element (each against a fresh
per-element scope), collects the per-element results into a list, and can **roll up** a
per-element field into a header total via `WithRollups`:

```go
inner, _ := pipe.NewPipeline([]pipe.Stage{ltvCheck}) // decides item.status per element
lines, _ := pipe.NewForEach("lines", "collateral", inner,
	pipe.WithForEachAs("item"),
	pipe.WithRollups(pipe.Rollup{Key: "approvedAmount", Agg: pipe.AggregateSum, As: "totalApproved"}),
)
// after Run:
firings := sc.FiringRulesFor("lines[2].check") // which rule decided line 3
total, _ := sc.GetFloat64("lines.totalApproved")
```

**Watch out:**

- Rolling up an **empty** collection leaves a `Sum`/`Min`/`Max` key **absent** (folding empty is
  undefined), while `Count` still writes an explicit `0` and `List` an empty slice.
- Nesting composes (`foreach` inside `foreach`), but a nested stage must bind its element under a
  **distinct `as` name** — reusing the outer name is `config.ErrForEachAsCollision`.
- Each element's firings and provenance are namespaced under `"<stage>[i]."`, so per-line
  explainability survives (`FiringRulesFor`, `Lineage`).

| Option | What it does | Quirk / default |
| --- | --- | --- |
| `WithForEachAs(name)` | Name each element is bound under in its per-element scope | Default `"item"`; must be distinct when nesting |
| `WithForEachOutput(key)` | Key (under the stage name) the results list is written to | Default `"items"` ⇒ `"<stage>.items"` |
| `WithRollups(…Rollup)` | Reduce a per-element `Key` across elements into `"<stage>.<As>"` | `Rollup{Key, Agg, As}`; empty `Key`/`As` ⇒ `ErrForEachEmptyRollup` |
| `WithForEachDependsOn(…)` | Cross-stage dependencies | See §8 |

See [`examples/09_pipe_foreach_test.go`](./examples/09_pipe_foreach_test.go).

### 8. Pipelines, the DAG & concurrency

A `Pipeline` takes a set of stages and figures out the order itself: it reads each stage's
declared `WithDependsOn` edges, **topologically sorts** them (Kahn), and validates the graph
once at construction — duplicate names, unknown dependencies, and cycles are caught *before* any
rule runs (a `*CycleError` even carries the concrete loop path). Turn on concurrency and
independent stages in the same dependency level run in parallel — with **identical** results:

```go
p, _ := pipe.NewPipeline([]pipe.Stage{subtotal, tax, total}, pipe.WithConcurrency())
// total declares depends-on tax; tax declares depends-on subtotal
_ = p.Run(context.Background(), sc) // runs subtotal → tax → total regardless of slice order
```

**Watch out:**

- Concurrency is a **performance knob, never a behavior change** — *provided the DAG is
  complete*. If a stage reads another stage's output but doesn't declare the edge, the two can
  land in the same level and race (expr reads a missing value as `nil`). Turning concurrency on
  is a good way to *surface* a latent missing dependency.
- `WithMaxParallel(n)` with `n < 1` is an `*InvalidMaxParallelError` at construction.
- Only top-level stages parallelize; a `foreach`'s inner pipeline stays sequential per element.

| Option | What it does | Quirk / default |
| --- | --- | --- |
| `WithConcurrency()` | Run each level's independent stages in parallel, unbounded | Default is sequential; result stays deterministic **iff** the DAG is complete |
| `WithMaxParallel(n)` | Same, capped at `n` concurrent stages | `n < 1` ⇒ `*InvalidMaxParallelError` |
| `WithRuleset(id)` | Stamp a `RulesetIdentity` onto every `Scope` this pipeline runs | Usually set for you by `config.Build` (see §10–11) |

See [`examples/08_pipe_pipeline_concurrency_test.go`](./examples/08_pipe_pipeline_concurrency_test.go).

### 9. Provenance & explainability

"Why was this application declined?" is a question every rule engine eventually has to answer.
Seed a `Scope` `pipe.WithProvenance()` and it records a `Derivation` for every seed input and
every stage write, so `Explain` can print a result's full lineage back to the raw inputs:

```go
sc := pipe.NewScope(map[string]any{"applicant": map[string]any{"score": 700}},
	pipe.WithProvenance())
_ = pipeline.Run(context.Background(), sc)
fmt.Print(sc.Explain("grade.limit"))
// grade.limit = 35000 [grade decision-table] expr: applicant.score * 50
//   applicant = map[score:700] [seed]
```

**Watch out:**

- Provenance is **opt-in** (there's a per-write cost) — without `WithProvenance`, `Explain`/
  `Lineage`/`Derivations` return empty.
- Pair `pipe.WithClock` with `pipe.NowFunc(clock)` registered as an expr `now()` host function
  so a temporal rule *and* `Scope.Duration`/`StageTimings` stay deterministic under test.

| Method | Answers | Quirk |
| --- | --- | --- |
| `Explain(path)` | A printable derivation tree for one output | Empty string unless `WithProvenance` |
| `Lineage(path)` | The `[]Derivation` chain behind a value | Empty without provenance |
| `Derivations()` | Every recorded `Derivation`, by path | — |
| `FiringRule(stage)` | The single/first rule a table fired | `(FiringRule, false)` if none |
| `FiringRulesFor(stage)` | Every rule a **collect**/**any** table (or a `foreach` element) fired | Use the `"<stage>[i]"` key for per-element |

See [`examples/10_pipe_provenance_clock_test.go`](./examples/10_pipe_provenance_clock_test.go).

### 10. Declarative config rulesets

Everything above can be *hand-wired in Go* — but the shape a ruleset usually ships in is a
single YAML or JSON document, so non-engineers can read it and it can live outside the binary.
`config.Parse` a provider, then `Build` it into the exact same `*pipe.Pipeline`:

```go
def, _ := config.Parse(context.Background(), config.FromYAMLString(`
schema:
  principal: 0
constants:
  aprRate: {$dec: "0.0725"}
stages:
  - name: interest
    type: single-expr
    expr: decimal(principal) * decimal(aprRate)
`))
pipeline, _ := def.Build()
```

**Watch out:**

- A constant declared `{$dec: "0.0725"}` is parsed as an **exact decimal**, and stays one
  through the whole pipeline — no float round-trip.
- A top-level `schema:` block **auto-enables strict** compilation for every stage — a field typo
  becomes a `*config.ConfigError` at `Build` (equivalently, `WithStrict()` demands a schema and
  errors if none is present). **Known limitation (W1):** strict-env currently only catches a
  typo used as a *bare identifier* (e.g. inside a `condition:`); a typo passed as an argument to
  a `decimal(...)` call is **not** yet flagged.
- `Lint()` (or `WithLintErrors()` to promote to a build error) catches missing-default and
  unreachable-rule smells.

| Provider | Source | Quirk |
| --- | --- | --- |
| `FromYAMLString` / `FromJSONString` | An in-memory string | Read on each `Parse`, no I/O in the provider |
| `FromYAMLBytes` / `FromJSONBytes` | An in-memory `[]byte` | Reusable |
| `FromReader(r, kind)` | A caller-owned `io.Reader` | Never closed by `Parse`, even if it's an `io.Closer` |
| `FromFile(path)` | A file, **format by extension** | `.yaml`/`.yml`/`.json`, else `ErrUnsupportedExtension`; no path confinement (trusted authoring) |
| `FromYAMLFile` / `FromJSONFile` | A file, format forced | Same trust boundary |
| `FromYAMLURL` / `FromJSONURL` | An HTTP(S) URL | 5 MiB / 10 s caps (tunable via `WithMaxBytes`/`WithHTTPClient`); **no SSRF confinement** — author-trusted only |

| Build option | What it does | Quirk / default |
| --- | --- | --- |
| `WithStrict()` | Require strict compile against the schema | Errors if no schema is available |
| `WithSchema(env)` | Supply/override the type-check env programmatically | Enables strict mode |
| `WithLintErrors()` | Promote every `Lint` finding to a `*LintError` at `Build` | Default: `Lint` is advisory only |
| `WithConcurrency()` / `WithMaxParallel(n)` | Build a concurrent pipeline (config-path §8) | `n < 1` ⇒ wrapped `*pipe.InvalidMaxParallelError` |
| `WithRulesetVersion(v)` | Set the author release label on the ruleset identity | Excluded from `Hash()` (see §11) |

See [`examples/11_config_rulesets_test.go`](./examples/11_config_rulesets_test.go).

### 11. Ruleset identity & replay

A decision you persist today may need to be re-explained months from now — *which exact ruleset
produced it?* `PipelineDef.Hash()` is a deterministic content fingerprint that `Build` stamps
onto every `Scope` it runs, so a serialized decision carries proof of its own provenance:

```go
sc := pipe.NewScope(map[string]any{"claimsScore": 85})
_ = pipeline.Run(context.Background(), sc)
b, _ := json.Marshal(sc) // persist; Unmarshal restores it losslessly

reloaded := &pipe.Scope{}
_ = json.Unmarshal(b, reloaded)
id, _ := reloaded.Ruleset()
def.MatchesRuleset(id) // true iff def is still the ruleset that produced this decision
```

**Watch out:**

- `Hash()` deliberately **excludes** the author's `version:` label — relabeling a release with
  no rule change leaves the hash unchanged, so identity tracks *content*, not naming.
- The `Scope` JSON is canonically **type-tagged** (`"v":2`), so a decimal reloads as a decimal
  and a sized int as `int64` — the persisted decision replays losslessly.

| API | What it gives you | Quirk |
| --- | --- | --- |
| `PipelineDef.Hash()` | Deterministic content fingerprint | Excludes `version` |
| `PipelineDef.MatchesRuleset(id)` | Whether a candidate def still matches a decision's identity | Compares hashes |
| `Scope.Ruleset()` | `(RulesetIdentity, bool)` stamped onto the scope | `false` if the pipeline had no identity |
| `config.WithRulesetVersion(v)` | Name the release without touching the hash | See §10 |

See [`examples/12_config_replay_test.go`](./examples/12_config_replay_test.go).

### 12. The untyped engine

`rlng.Engine` is the facade that removes the per-call boilerplate — seed a `Scope`, `Run` the
pipeline, hand back the result — so calling a ruleset is one line. Construct it from a built
pipeline, or go straight from YAML in a single call:

```go
engine, _ := rlng.New(pipeline, rlng.WithScopeOptions(pipe.WithProvenance()))
out, _ := engine.Evaluate(context.Background(),
	map[string]any{"score": 720, "balance": 1200, "limit": 5000})
// out["offer"].(map[string]any)["apr"]

engine2, _ := rlng.NewFromYAML(context.Background(), yamlDoc) // parse + build + New, one call
```

**Watch out:**

- `Evaluate` returns the plain `map[string]any`; `EvaluateScope` returns the live `*pipe.Scope`
  for when you need timing, JSON persistence, or `Explain`.
- Concurrency options only work on the **config** constructors (concurrency is baked in at
  `Build`); passing `rlng.WithConcurrency()` to `rlng.New` returns
  `rlng.ErrConcurrencyRequiresConfig`.

| Constructor / option | What it does | Quirk |
| --- | --- | --- |
| `rlng.New(pipeline, …)` | Wrap an already-built pipeline | `nil` pipeline ⇒ `ErrNilPipeline` |
| `rlng.NewFromYAML(ctx, yaml, …)` | Parse + build + wrap, one call | For JSON/file/URL use `NewFromProvider` |
| `rlng.NewFromProvider(ctx, provider, …)` | Same, from any `config.Provider` | Build-time options need explicit `Parse`→`Build`→`New` |
| `WithScopeOptions(…pipe.ScopeOption)` | Configure the per-`Evaluate` scope | e.g. `pipe.WithProvenance()`, `pipe.WithStrict()` |
| `WithConcurrency()` / `WithMaxParallel(n)` | Build concurrently (`NewFrom*` only) | On `New` ⇒ `ErrConcurrencyRequiresConfig` |

See [`examples/13_engine_untyped_test.go`](./examples/13_engine_untyped_test.go).

### 13. The typed engine & result mapping

The top of the ladder: `rlng.TypedEngine[I, R]` pairs an `Engine` with a `Mapper[R]` so
`Evaluate` takes a typed struct **in** and returns a typed struct **out**, with a
`decimal.Decimal` field surviving both boundaries exactly. The mapper projects scope paths onto
your result fields via a `MappingTemplate`:

```go
mapper, _ := rlng.NewMapper[LoanQuote](rlng.MappingTemplate{"fee": "fee"})
engine, _ := rlng.NewTypedEngine[LoanApplication, LoanQuote](pipeline, mapper)

quote, _ := engine.Evaluate(context.Background(), LoanApplication{
	Principal: decimal.NewFromInt(1_000),
	Rate:      decimal.RequireFromString("0.0725"),
})
fmt.Println(quote.Fee.StringFixed(2)) // 72.50
```

**Watch out:**

- Input handling has sharp edges by design: a `nil` (or nil-pointer) input is `ErrNilInput`, but
  an **empty map is a valid** seed; a `decimal.Decimal` struct field is restored into the seed
  intact (mapstructure would otherwise flatten it away).
- Mapping a **fractional** decimal into an `int` result field is **refused, not truncated** — a
  `*rlng.MappingError` wrapping `rlng.ErrLossyResultNarrowing`. It must be integral and in range.

| API | What it does | Quirk |
| --- | --- | --- |
| `NewMapper[R](MappingTemplate)` | Compile a `path → result-field` projection | `MappingTemplate` is `map[string]string` (field → scope expr) |
| `NewTypedEngine[I, R](pipeline, mapper, …)` | Wrap a built pipeline + mapper | `nil` mapper ⇒ `ErrNilMapper` |
| `NewTypedFromYAML[I, R](ctx, yaml, mapper, …)` | Parse + build + wrap typed, one call | Also `NewTypedFromProvider` for JSON/file/URL |
| `(*TypedEngine).Evaluate(ctx, in)` | Typed in → typed out | Decimal→`int` field must be integral & in range, else `ErrLossyResultNarrowing` |

See [`examples/14_engine_typed_test.go`](./examples/14_engine_typed_test.go).

Runnable versions of every snippet live in [`examples/`](./examples) (start at
[`examples/doc.go`](./examples/doc.go) for the full index) and the package `Example` tests —
run them with `go test ./...`.

## Installation

Not yet released. Once the module is published and a `vX.Y.Z` tag is cut:

```bash
go get github.com/kartaladev/rlng@latest
```

Requires **Go 1.25+**.

## Development

Contributor conventions — TDD, the skill-driven workflow, documentation/ADR requirements,
commit discipline, and the library quality gates — are documented in
[`CLAUDE.md`](./CLAUDE.md). Common commands:

```bash
go build ./...
go test ./... -race
go vet ./...
golangci-lint run ./...
govulncheck ./...
```

## Releases

Releases are **tag-driven and SemVer'd**. Pushing an annotated tag `vX.Y.Z` (e.g. `v0.0.1`)
triggers the [`release`](./.github/workflows/release.yml) GitHub Action, which publishes a
**GitHub Release** with auto-generated notes. As a library there are no binaries to
build — the tag itself is the distribution. Consumers pin a version with
`go get github.com/kartaladev/rlng@vX.Y.Z`. Breaking changes to the exported API require a
major-version bump and a recorded ADR.

## License

Licensed under the [Apache License 2.0](./LICENSE).
