---
title: Code Style — Mechanical Rules
summary: Mechanically detectable language constructs that Landing does not accept.
owner: pattern-author
tier: lint
group: language
candidates: "**/*.go"
---

# Code style — mechanical rules

## Rules

- **R1. NEVER** use package-level mutable state. Pass dependencies explicitly
  or construct them at a visible boundary so tests and execution order remain
  reliable.
- **R2. NEVER** use a naked return. Name the value returned at each exit so the
  path's result is visible without tracking mutable named return values.
- **R3. NEVER** use `init()` for product behavior. Explicit construction and
  registration make initialization order visible and testable.
- **R4. MUST** put invalid-state guards first and return an error or zero result
  before the main operation. The valid path remains readable without nested
  indentation.

Generated and vendored sources are excluded.

```go
// Wrong
var currentRoute Route

func chooseRoute(routes []Route) (route Route, err error) {
	if len(routes) == 0 {
		err = ErrNoRoutes
		return
	}
	route = routes[0]
	return
}

// Right
func chooseRoute(routes []Route) (Route, error) {
	if len(routes) == 0 {
		return Route{}, ErrNoRoutes
	}

	return routes[0], nil
}
```
