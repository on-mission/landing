---
title: Domain Ownership and Naming
summary: A module owns one recognizable product concept whose members change for one cause.
owner: pattern-author
tier: packet
group: architecture
---

# Domain ownership and naming

## Rules

- **R1. MUST** name modules and public exports after the product concept they
  own, not the procedure used to produce a value.
- **R2. MUST** be able to name the single cause that changes a module's public
  members together. Without one shared cause, the module is a theme bucket.
- **R3. NEVER** create public `utils`, `helpers`, `common`, `misc`, or `shared`
  modules. Re-home behavior with the concept whose change would require it.
- **R4. MUST** keep procedural helpers private inside the owning module. The
  file boundary exports the domain contract, not its assembly steps.
- **R5. PREFER** product vocabulary over provider vocabulary outside adapter
  boundaries. A caller asks for a route or tier, not an Anthropic/OpenAI-shaped
  operation.
- **R6. MUST** separate deterministic policy from operations that perform I/O or
  depend on time. Convenience is not an ownership test.
- **R7. NEVER** introduce a `manager` merely to collect verbs. A stateful service
  or manager must own a real lifecycle or domain resource.

## Review test

Ask “what is the thing?” and “what single event changes all of this?” If neither
has a precise answer, fix ownership before debating function size.
