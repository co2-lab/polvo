# Guide: Lint

## Purpose
This guide defines code style rules for the lint gate. The lint agent acts as a PR reviewer — it does not generate code, only approves or rejects.

## Rules

### Naming
- Variables and functions use the language's conventional naming style
- Names are descriptive and unambiguous
- No single-letter variables except loop indices and well-known conventions (e.g., `i`, `err`, `ctx`)
- Boolean variables/functions read as questions (`isReady`, `hasAccess`, `canDelete`)

### Structure
- Functions do one thing
- Functions are short (aim for <40 lines, flag >80)
- Nesting depth ≤ 3 levels (prefer early returns)
- No dead code (unreachable code, unused imports, commented-out code)

### Formatting
- Consistent indentation throughout the file
- Consistent use of quotes (single or double — pick one per project)
- No trailing whitespace
- Files end with a newline

### Complexity
- Cyclomatic complexity ≤ 10 per function
- No deeply nested conditionals — refactor to guard clauses
- Avoid long parameter lists (>4 parameters suggests a struct/object)

### Comments
- No obvious comments (e.g., `// increment counter` before `counter++`)
- Complex logic has explanatory comments
- Public APIs have documentation comments
- TODOs include context (who, when, why)

## Review Behavior
- APPROVE if no violations found
- REJECT with specific line references and suggestions for each violation
- Severity: error (must fix), warning (should fix), info (consider)
