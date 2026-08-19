#!/usr/bin/env node

import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const metadataPath = resolve(projectRoot, ".codex/skill-meta.json");
const lockPath = resolve(projectRoot, ".codex/skill-sync.lock.json");
const checkOnly = process.argv.includes("--check");

const hash = (content) =>
  createHash("sha256").update(content).digest("hex");

const readText = async (path) => readFile(path, "utf8");

const renderClaudeAgent = (name, description, body) => `---
name: ${name}
description: ${JSON.stringify(description)}
---

${body.trim()}\n`;

const renderCodexSkill = (name, description, body) => `---
name: ${name}
description: ${JSON.stringify(description)}
---

${body.trim()}\n`;

const buildRole = async ([name, metadata]) => {
  const sourcePath = resolve(projectRoot, metadata.source);
  const body = await readText(sourcePath);
  const claudePath = resolve(projectRoot, `.claude/agents/${name}.md`);
  const codexPath = resolve(projectRoot, `.codex/skills/${name}/SKILL.md`);

  return {
    name,
    source: metadata.source,
    sourceHash: hash(body),
    outputs: [
      {
        path: claudePath,
        relativePath: `.claude/agents/${name}.md`,
        content: renderClaudeAgent(name, metadata.description, body),
      },
      {
        path: codexPath,
        relativePath: `.codex/skills/${name}/SKILL.md`,
        content: renderCodexSkill(name, metadata.description, body),
      },
    ],
  };
};

const fileMatches = async ({ path, content }) => {
  try {
    return (await readText(path)) === content;
  } catch (error) {
    if (error?.code === "ENOENT") return false;
    throw error;
  }
};

const writeOutput = async ({ path, content }) => {
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, content, "utf8");
};

const main = async () => {
  const metadataText = await readText(metadataPath);
  const metadata = JSON.parse(metadataText);
  const roles = await Promise.all(Object.entries(metadata).map(buildRole));
  const outputs = roles.flatMap(({ outputs: roleOutputs }) => roleOutputs);
  const lock = {
    version: 1,
    metadataHash: hash(metadataText),
    roles: Object.fromEntries(
      roles.map(({ name, source, sourceHash, outputs: roleOutputs }) => [
        name,
        {
          source,
          sourceHash,
          outputs: roleOutputs.map(({ relativePath, content }) => ({
            path: relativePath,
            hash: hash(content),
          })),
        },
      ]),
    ),
  };
  const lockOutput = {
    path: lockPath,
    relativePath: ".codex/skill-sync.lock.json",
    content: `${JSON.stringify(lock, null, 2)}\n`,
  };
  const expectedOutputs = [...outputs, lockOutput];

  if (checkOnly) {
    const checks = await Promise.all(expectedOutputs.map(fileMatches));
    const stale = expectedOutputs.filter((_, index) => !checks[index]);

    if (stale.length === 0) {
      process.stdout.write("Agent surfaces are synchronized.\n");
      return;
    }

    stale.forEach(({ relativePath }) =>
      process.stderr.write(`Stale generated surface: ${relativePath}\n`),
    );
    process.exitCode = 1;
    return;
  }

  await Promise.all(expectedOutputs.map(writeOutput));
  process.stdout.write(`Synchronized ${roles.length} agent roles.\n`);
};

await main();
