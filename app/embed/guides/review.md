# Guide: Review

## Purpose
This guide defines the criteria for the review agent, which coordinates PR approval. It performs the general review after lint and best-practices gates pass.

## Review Criteria

### Spec Adherence
- Changes align with the spec (`*.spec.md`)
- All spec requirements addressed by the PR are correctly implemented
- No spec requirements are contradicted

### Coherence
- Changes are internally consistent
- New code integrates well with existing codebase
- No conflicting patterns or conventions introduced

### Completeness
- The PR does what it claims to do (title and description match the changes)
- No partial implementations left without TODOs or follow-up issues
- Error handling is complete

### Quality
- Code is readable and maintainable
- Changes are minimal — no unnecessary modifications
- Performance implications are considered

### Testing Impact
- If behavior changed, corresponding tests should be updated
- No test regressions introduced

## Process
1. Verify lint gate passed
2. Verify best-practices gate passed
3. Review the full diff against the criteria above
4. Decision:
   - **APPROVE**: all criteria met, merge the PR
   - **REJECT**: explain what needs to change with specific references

## Output Format
The review must include:
- Summary: one-paragraph assessment
- Findings: list of issues (if any) with file, line, and suggestion
- Decision: APPROVE or REJECT
