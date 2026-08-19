---
name: tldr
description: "Produce a human-level architectural walkthrough of the solution currently being designed."
---

# TL;DR walkthrough

Explain the solution currently being designed in this conversation at a human,
architectural, implementation-aware level. Do not design a new solution or make
changes.

## Reconstruct

Use the current conversation first, then any named plan or issue, then the real
files and instructions the solution touches. Read artifacts rather than
reconstructing code from memory.

If the proposed solution is contradictory or incomplete, say so in one line and
walk through the most coherent current reading.

## Output

### 1. The problem

Two to four sentences naming the user problem and known root cause.

### 2. The solution in one breath

One short end-to-end paragraph.

### 3. End-to-end workflow

A numbered flow from input through the real components to the result. Name files,
functions, commands, or prompts when they exist and explain where behavior
changes.

### 4. Load-bearing changes

Only the code, contracts, configuration, or prompts that make the solution work.
Use short real excerpts with `path:line` references; never invent a proposed
snippet that is not recorded in an artifact.

### 5. Why it works and what to watch

Explain how the solution addresses the cause and name sharp edges, assumptions,
or unresolved decisions.

Keep the walkthrough tight. If no code or prompt artifact exists yet, say so
instead of manufacturing one. Do not edit files, write a plan, or implement.
