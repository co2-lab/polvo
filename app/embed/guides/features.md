# Guide: Features

## Purpose
This guide defines how feature scenarios should be written in Gherkin format. The features agent generates `*.feature` files from specs.

## Format
Use standard Gherkin syntax:
```gherkin
Feature: <feature name>
  As a <role>
  I want <goal>
  So that <benefit>

  Scenario: <scenario name>
    Given <precondition>
    When <action>
    Then <expected outcome>
```

## Rules

### Coverage
- Every functional requirement in the spec MUST have at least one scenario
- Happy path scenarios come first
- Error/edge case scenarios follow

### Naming
- Feature name matches the interface name
- Scenario names are descriptive and unique
- Use present tense

### Steps
- Given: set up preconditions (state, data)
- When: describe the single action being tested
- Then: assert observable outcomes
- Use And/But for additional steps within a block

### Data
- Use Scenario Outline + Examples for parameterized scenarios
- Keep example data realistic but minimal
- Use background for shared preconditions

## Quality Criteria
- Scenarios are independent — no ordering dependency
- Each scenario tests one behavior
- No implementation details in steps
- Steps are reusable across scenarios
