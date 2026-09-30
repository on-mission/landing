# Documentation

Landing keeps a small documentation surface with one home for each kind of
decision.

## Product

Product documentation is the primary source of truth. Read it in this order:

1. [Product definition](product/main.md) — value, users, experience, boundaries,
   and core capabilities
2. [User stories](product/user-stories.md) — outcomes the product must support
3. [Onboarding](product/onboarding.md) — the path from discovery to a usable
   project setup
4. [Routing](product/routing.md) — the user-facing policy, default tiers, and
   customization model
5. [Messaging](product/messaging.md) — category, public language, and claims to
   avoid
6. [Messages](product/messages.md) — how chats reach one another by name and
   exchange durable messages

## Engineering context

- [Architecture structure](architecture/STRUCTURE.md) defines what belongs in
  architecture documentation.
- [Architecture canvas](architecture/canvas.md) shows the system at the
  component-and-contract level.
- [Patterns](patterns/overview.md) indexes the binding Go, reliability, testing,
  domain, and integration rules.
- [Agent surfaces runbook](runbooks/agent-surfaces.md) covers docs search and
  author-role synchronization.

There is intentionally no `technical/` catch-all directory and no prose CLI
reference. Implementation detail belongs in code; volatile CLI behavior belongs
in code-owned help and tests.
