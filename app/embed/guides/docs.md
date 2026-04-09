# Guide: Docs

## Purpose
This guide defines how documentation should be generated and maintained. The docs agent synthesizes documentation from specs and features.

## What to Document
- Public interfaces: inputs, outputs, behavior
- Setup and configuration instructions
- Usage examples for each interface
- Error codes and their meanings

## Structure
- Start with a one-line summary
- Follow with a detailed description
- Include code examples where helpful
- Group related interfaces together

## Rules
- Documentation must match the current spec and implementation
- Use clear, concise language — avoid jargon
- Keep examples minimal but complete (copy-paste runnable)
- Update existing docs rather than creating duplicates
- Remove documentation for deleted interfaces

## Sources
The docs agent reads from:
1. `*.spec.md` — for requirements and behavior descriptions
2. `*.feature` — for usage scenarios and examples
3. Interface files — for API signatures and types

## Quality Criteria
- Every public interface is documented
- No stale documentation (mismatches with current spec)
- Examples are correct and up-to-date
- Documentation is navigable (table of contents for long docs)
