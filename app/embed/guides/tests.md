# Guide: Tests

## Purpose
This guide defines testing standards. The tests agent generates `*.test.*` files from feature scenarios.

## Test Types
- **Unit tests**: test individual functions/methods in isolation
- **Integration tests**: test interactions between components
- **Contract tests**: verify interface contracts (inputs/outputs)

## Naming Convention
- Test functions: `Test<Interface>_<Scenario>` or `test_<interface>_<scenario>`
- Test files: co-located with the interface file, same name with `.test.` suffix

## Structure (Arrange-Act-Assert)
```
1. Arrange: set up test data and dependencies
2. Act: call the function/method under test
3. Assert: verify the expected outcome
```

## Rules
- One assertion concept per test (multiple asserts OK if testing one behavior)
- Every Gherkin scenario maps to at least one test
- Tests must be deterministic — no random data, no time dependency
- Mock external dependencies, not internal logic
- Test error paths, not just happy paths

## Coverage
- All scenarios from `*.feature` must have corresponding tests
- Edge cases from the spec must be covered
- Aim for behavioral coverage, not line coverage

## Quality Criteria
- Tests run independently and in any order
- No shared mutable state between tests
- Test names clearly describe what is being tested
- Failed tests produce clear error messages
