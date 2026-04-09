# Guide: Spec

## Purpose
This guide defines how specification documents (`*.spec.md`) should be structured. It serves as a template for the spec agent to generate and validate specs.

## Required Sections

### 1. Overview
- One-paragraph summary of what this interface does
- Who uses it and why

### 2. Requirements
- Numbered list of functional requirements
- Each requirement must be testable and unambiguous
- Use "MUST", "SHOULD", "MAY" per RFC 2119

### 3. Interface Contract
- Inputs: parameters, types, constraints
- Outputs: return types, status codes, error formats
- Side effects: state changes, events emitted

### 4. Behavior
- Normal flow description
- Edge cases and error handling
- Concurrency and ordering guarantees (if applicable)

### 5. Dependencies
- Other interfaces or services this depends on
- External systems or APIs

### 6. Non-Functional Requirements
- Performance constraints (latency, throughput)
- Security considerations
- Accessibility requirements (if UI)

## Quality Criteria
- Every requirement must be verifiable by a test
- No implementation details — describe WHAT, not HOW
- Use concrete examples for complex behaviors
- Keep language precise and consistent
