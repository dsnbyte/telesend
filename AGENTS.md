# Telesend

## Workflow Behavior

### Think Before Coding
- State assumptions explicitly; ask if uncertain.
- Present multiple interpretations instead of silently picking one.
- Flag simpler alternatives and push back when warranted.
- Stop and name the confusion if something is unclear.

### Simplicity First
- Write the minimum code that solves the problem — no speculative features, abstractions, configurability, or error handling for impossible cases.
- Rewrite shorter wherever possible.
- Test: would a senior engineer call this overcomplicated?

### Surgical Changes
- Touch only what the task requires — no unrelated refactors, style edits, or "improvements."
- Match existing style even when you'd do it differently.
- Test: every changed line must trace to the request.

## Code Style
Write code that is accessible, performant, type-safe, and maintainable. Favor clarity and explicit intent over brevity.
