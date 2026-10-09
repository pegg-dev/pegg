# Coding Rules

## Investigation Phase

1. **Always use `enterplanmode`** before any investigation or non-trivial implementation.
2. **Research first**: Explore the codebase, read relevant files, and understand the existing patterns.
3. **Always use `exitplanmode`** before starting any implementation. Present the plan for approval.

## Implementation Phase

1. **Create tasks** using `todowrite` before starting implementation.
2. **Use subagents** for parallelizable work. Delegate independent tasks to background subagents.
3. **Track progress** by updating todo status as you work.

## Post-Implementation Checks

After finishing implementation, always run:

```bash
make fmt   # Format code
make vet   # Run go vet
make test  # Run tests
```

If any check fails, fix the issues before considering the work complete.
