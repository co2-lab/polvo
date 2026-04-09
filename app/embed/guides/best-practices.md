# Guide: Best Practices

## Purpose
This guide defines architectural patterns and best practices. The best-practices agent acts as a PR reviewer — it does not generate code, only approves or rejects.

## Patterns to Follow

### Error Handling
- Handle errors explicitly — no silent swallowing
- Wrap errors with context when propagating
- Use typed/sentinel errors for expected failure modes
- Fail fast on unrecoverable errors

### Separation of Concerns
- Business logic is separated from I/O
- No direct database/HTTP calls in business logic
- Dependencies are injected, not created inline

### Immutability
- Prefer immutable data structures where practical
- Don't mutate function arguments
- Return new values instead of modifying in place

### API Design
- Consistent naming across endpoints/methods
- Use appropriate HTTP methods and status codes
- Validate inputs at system boundaries
- Return structured errors with codes and messages

### Security
- Never log sensitive data (passwords, tokens, PII)
- Validate and sanitize all external input
- Use parameterized queries, never string concatenation
- Principle of least privilege for permissions

## Anti-Patterns to Flag
- God objects/functions (too many responsibilities)
- Premature abstraction (abstractions without multiple consumers)
- Magic numbers/strings (use named constants)
- Circular dependencies
- Tight coupling to external services without abstraction

## Review Behavior
- APPROVE if no violations found
- REJECT with pattern reference and concrete suggestion for each violation
