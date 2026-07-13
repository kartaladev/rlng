// 11 — config declarative rulesets: everything shown in files 01–10 as Go
// constructor calls (expr.NewPredicate, pipe.NewDecisionTable, ...) can
// instead be authored as one YAML or JSON document and turned into a
// *pipe.Pipeline by config.Parse + PipelineDef.Build — the shape a rule
// actually ships in. This file shows the document-authoring surface: the
// YAML/JSON shorthand-vs-object duality, pipeline-level constants (including
// the exact-decimal $dec literal), a strict schema that turns a field typo
// into a Build-time error, and Lint's authoring-smell checks.
package examples_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
)

// Example_rulesetYAMLJSONParity authors the SAME two-stage order-pricing
// pipeline twice — once as YAML using the scalar shorthand (a bare string is
// the expression), once as JSON using the object form ({"expr": "..."}) —
// and shows both parse, build, and evaluate to an identical result. This is
// the central config quirk: YAML's scalar-shorthand accepts an UNQUOTED
// scalar (expr: price * quantity), but JSON syntax has no unquoted-string
// token at all, so the shorthand's value must always be a quoted JSON string
// ("expr": "price * quantity"); a bare JSON number or boolean there is
// rejected rather than silently stringified.
func Example_rulesetYAMLJSONParity() {
	const yamlDoc = `
stages:
  - name: subtotal
    type: single-expr
    expr: price * quantity
  - name: total
    type: single-expr
    expr: subtotal * 1.08
    depends_on: [subtotal]
`
	// The object form spells out the same "expr" field explicitly; it is the
	// form to reach for when a stage also needs fallback/globals/coerce.
	const jsonDoc = `{
  "stages": [
    {"name": "subtotal", "type": "single-expr", "expr": {"expr": "price * quantity"}},
    {"name": "total", "type": "single-expr", "expr": {"expr": "subtotal * 1.08"}, "depends_on": ["subtotal"]}
  ]
}`

	yamlDef, err := config.Parse(context.Background(), config.FromYAMLString(yamlDoc))
	if err != nil {
		fmt.Println("yaml parse:", err)
		return
	}
	jsonDef, err := config.Parse(context.Background(), config.FromJSONString(jsonDoc))
	if err != nil {
		fmt.Println("json parse:", err)
		return
	}

	yamlPipeline, err := yamlDef.Build()
	if err != nil {
		fmt.Println("yaml build:", err)
		return
	}
	jsonPipeline, err := jsonDef.Build()
	if err != nil {
		fmt.Println("json build:", err)
		return
	}

	seed := map[string]any{"price": 250, "quantity": 4}

	yamlScope := pipe.NewScope(seed)
	if err := yamlPipeline.Run(context.Background(), yamlScope); err != nil {
		fmt.Println("yaml run:", err)
		return
	}
	jsonScope := pipe.NewScope(seed)
	if err := jsonPipeline.Run(context.Background(), jsonScope); err != nil {
		fmt.Println("json run:", err)
		return
	}

	yamlTotal, _ := yamlScope.Get("total")
	jsonTotal, _ := jsonScope.Get("total")
	fmt.Println("yaml total:", yamlTotal)
	fmt.Println("json total:", jsonTotal)
	fmt.Println("identical:", yamlTotal == jsonTotal)

	// Output:
	// yaml total: 1080
	// json total: 1080
	// identical: true
}

// Example_rulesetConstantsAndStrictSchema shows two ruleset-authoring
// safeguards used together. A constants: block declares a pipeline-level
// named value once — here an exact-decimal APR rate via the {$dec: "..."}
// literal marker, which Build hydrates into a decimal.Decimal before any
// stage compiles, so every stage sees aprRate as an overridable `??` default
// (see file 03's globals-precedence quirk). A top-level schema: block
// declares the expected input shape; its mere PRESENCE auto-enables strict
// compilation (expr.WithEnv) for every stage in the document — no explicit
// config.WithStrict() Build option needed — so a field-name typo is rejected
// at Build as a *config.ConfigError instead of silently evaluating to nil at
// runtime (the same guard file 03 demonstrates at the expr layer, now
// enforced document-wide).
func Example_rulesetConstantsAndStrictSchema() {
	const correct = `
schema:
  principal: 0
constants:
  aprRate: {$dec: "0.0725"}
stages:
  - name: interest
    type: single-expr
    condition: "principal > 0"
    expr: decimal(principal) * decimal(aprRate)
`
	def, err := config.Parse(context.Background(), config.FromYAMLString(correct))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	pipeline, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}
	sc := pipe.NewScope(map[string]any{"principal": 10_000})
	if err := pipeline.Run(context.Background(), sc); err != nil {
		fmt.Println("run:", err)
		return
	}
	interest, _ := sc.Get("interest")
	fmt.Println("interest on $10,000 at the constant APR:", interest)

	// "princpal" is a typo of "principal" in the condition gate. Against a
	// schema-less definition this would compile lenient (the gate would
	// always evaluate falsy against an undefined name, silently skipping the
	// stage); here the schema makes it a Build-time failure instead.
	const typoed = `
schema:
  principal: 0
constants:
  aprRate: {$dec: "0.0725"}
stages:
  - name: interest
    type: single-expr
    condition: "princpal > 0"
    expr: decimal(principal) * decimal(aprRate)
`
	def2, err := config.Parse(context.Background(), config.FromYAMLString(typoed))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	_, err = def2.Build()
	var cfgErr *config.ConfigError
	fmt.Println("typo rejected at Build:", errors.As(err, &cfgErr))

	// Output:
	// interest on $10,000 at the constant APR: 725
	// typo rejected at Build: true
}

// Example_rulesetLintFindings shows Lint's two authoring-smell checks on
// first-match decision tables (hit_policy single/unique — collect and any
// are exempt, see file 07's aggregation example: collect legitimately
// produces zero matches, and any legitimately allows overlap by design). A
// missing-default table has neither a catch-all rule nor a default: block,
// so an unmatched input silently produces no output. An unreachable-rule
// table has a later rule shadowed by an earlier unconditional catch-all
// (condition: "true"), so the later rule can never fire. Lint is advisory by
// default — Build succeeds regardless of a finding — but WithLintErrors
// promotes every finding to a Build-time *config.LintError, letting a CI
// pipeline fail fast on either smell. isCatchAll detection is deliberately
// syntactic and best-effort (only the literal true or the trivial tautology
// 1 == 1): a semantically-always-true condition written another way (e.g.
// score >= 0) is NOT recognized, so it may still be (falsely) flagged
// missing-default — Lint is a smell detector, not a proof of coverage.
func Example_rulesetLintFindings() {
	const noDefault = `
stages:
  - name: eligibility
    type: decision-table
    rules:
      - id: HIGH_INCOME
        condition: "income >= 100000"
        decisions:
          approved: "true"
      - id: MED_INCOME
        condition: "income >= 50000"
        decisions:
          approved: "true"
`
	missingDefaultDef, err := config.Parse(context.Background(), config.FromYAMLString(noDefault))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	findings := missingDefaultDef.Lint()
	fmt.Println("missing-default findings:", len(findings))
	if len(findings) > 0 {
		fmt.Println("code:", findings[0].Code)
	}

	const shadowed = `
stages:
  - name: shipping
    type: decision-table
    rules:
      - id: STANDARD
        condition: "true"
        decisions:
          fee: "5"
      - id: EXPRESS
        condition: "express"
        decisions:
          fee: "15"
`
	unreachableDef, err := config.Parse(context.Background(), config.FromYAMLString(shadowed))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	findings2 := unreachableDef.Lint()
	fmt.Println("unreachable-rule findings:", len(findings2))
	if len(findings2) > 0 {
		fmt.Println("code:", findings2[0].Code)
	}

	// Build is advisory-only by default: the shadowed rule does not block
	// construction.
	if _, err := unreachableDef.Build(); err != nil {
		fmt.Println("advisory build:", err)
	} else {
		fmt.Println("advisory build: succeeded despite the finding")
	}

	// WithLintErrors promotes the same finding to a Build-time error.
	_, err = unreachableDef.Build(config.WithLintErrors())
	var lintErr *config.LintError
	fmt.Println("WithLintErrors rejects it:", errors.As(err, &lintErr))

	// Output:
	// missing-default findings: 1
	// code: missing-default
	// unreachable-rule findings: 1
	// code: unreachable-rule
	// advisory build: succeeded despite the finding
	// WithLintErrors rejects it: true
}
