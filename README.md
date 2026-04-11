# Chiperka

**The missing manifest standard for AI-assisted backend development.**
A declarative file format your team writes once, so AI agents and humans both know how to run, exercise, and debug your service.

---

## The problem

Your AI agent reads code. It does not run it.

That means Cursor, Claude Code, and every other coding agent guesses what your backend actually does at runtime. They hallucinate endpoints, invent response shapes, and miss the one container that needs to be up before anything works. The truth lives in your `Makefile`, your `docker-compose.yml`, your CI config, the team Wiki, and three Slack threads from last quarter — never in one place an agent can read.

## The fix

You commit `.chiperka` files into your repo. Each one is a small, declarative YAML document that describes one scenario your project supports — what services it needs, how to bring them up, what request to send, what response to expect. Together, the `.chiperka` files in a repo form a **runnable specification** of what your project does.

Then you run `chiperka mcp`. That starts a Model Context Protocol server that exposes those scenarios as structured tools to any MCP-compatible agent. Your agent stops guessing. It lists the scenarios, runs them against real Docker services, reads the actual output, and answers questions with facts.

> MCP (Model Context Protocol) is the open standard Claude Code, Cursor, Claude Desktop, and Continue use to call external tools. If you've never set up an MCP server before, the section below has you covered.

---

## What a `.chiperka` file looks like

Drop this anywhere in your repo as `tests/login.chiperka` (or whatever path makes sense):

```yaml
name: auth
tests:
  - name: login-with-valid-credentials
    tags: [smoke, auth]
    services:
      - name: api
        image: ghcr.io/myorg/api:latest
        environment:
          DB_URL: postgres://db:5432/test
        healthcheck:
          test: "curl -f http://localhost:8080/health"
          retries: 30
    setup:
      - http:
          target: http://api:8080
          request:
            method: POST
            url: /seed
    execution:
      target: http://api:8080
      request:
        method: POST
        url: /auth/login
        headers:
          Content-Type: application/json
        body: '{"email": "test@example.com", "password": "hunter2"}'
    assertions:
      - response:
          statusCode: 200
```

A real project usually has many such files — `auth.chiperka`, `users.chiperka`, `billing.chiperka`, one per area. The whole corpus is your specification. Chiperka discovers them automatically by walking the repo.

You can also drop a small `.chiperka/chiperka.yaml` in the repo root for **CLI configuration** — execution variables and similar cross-run settings. It's optional, and it does not declare any project resources. Services and tests both live in `.chiperka` files; `chiperka.yaml` is just for the runner.

---

## Connect your AI agent

### Claude Code

Add to `.mcp.json` in your project:

```json
{
  "mcpServers": {
    "chiperka": {
      "command": "chiperka",
      "args": ["mcp", "--configuration", ".chiperka/chiperka.yaml"]
    }
  }
}
```

### Cursor

Add to `~/.cursor/mcp.json` (or per-project `.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "chiperka": {
      "command": "chiperka",
      "args": ["mcp", "--configuration", ".chiperka/chiperka.yaml"]
    }
  }
}
```

### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "chiperka": {
      "command": "chiperka",
      "args": ["mcp"]
    }
  }
}
```

The `--configuration` flag is optional — it just tells Chiperka where the shared CLI config lives, if you have one. Restart your client after adding the server. Your agent now has a `chiperka` tool group and can introspect, run, and observe your backend on its own.

---

## What the agent can do

Once connected, the agent can call:

- **`chiperka_context`** — get the AI-readable tool reference and project overview
- **`chiperka_list`** — discover every `.chiperka` file the repo contains
- **`chiperka_read`** — read a `.chiperka` file as structured JSON, so the agent knows exactly what services and assertions it declares
- **`chiperka_validate`** — validate a file without running it
- **`chiperka_execute`** — run an inline scenario provided by the agent itself (great for "what if I send this?" experiments)
- **`chiperka_run`** — execute one or more `.chiperka` files end-to-end and return structured results

Every run produces a structured record the agent can reference in later calls, so it doesn't have to re-burn context tokens reasoning about something it already saw.

---

## Install

**Homebrew**

```bash
brew tap chiperka/tap
brew install chiperka
```

**Script**

```bash
curl -fsSL https://raw.githubusercontent.com/chiperka/chiperka/main/install.sh | sh
```

**Docker**

```bash
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v ./:/code:delegated \
  finie/chiperka:latest chiperka mcp
```

Requires Docker on the host. Chiperka orchestrates real containers — no mocks, no stubs.

---

## How it works

```
1. Someone on your team writes a few .chiperka files describing real
   scenarios the service supports, and commits them to the repo.
2. Anyone who clones the repo runs `chiperka mcp`.
3. Their AI agent connects via MCP, discovers every .chiperka file, and
   reads them as structured data.
4. When the user asks a question, the agent picks the relevant scenarios,
   runs them against real Docker services, and reads the actual output.
5. The agent answers with facts from a real run, not guesses from static
   code reading.
```

The files are **manually written**, live in **git**, and are **reviewed in PRs**. No auto-discovery. No magic. One person on the team writes them; everyone — humans and agents — benefits forever.

---

## Why a standard

Every developer tool that became infrastructure was a manually-written declarative file:

- `package.json` for Node
- `Dockerfile` for container builds
- `Cargo.toml` for Rust
- `pyproject.toml` for Python
- `openapi.yaml` for HTTP APIs

None of them are magic. None of them auto-discover. They are stable, version-controlled, review-able, and tools build on top of them. `.chiperka` files are the same idea, scoped to "what does this service do at runtime, and how do you exercise it."

---

## Status

Chiperka is in active early development. The CLI runs, the MCP server runs, and the `.chiperka` file format is being stabilized through dogfooding. Expect breaking changes to the format before `v1.0` — pin a version if you need stability.

- License: Apache-2.0
- Source: https://github.com/chiperka/chiperka
- Specification: https://github.com/chiperka/specification
- Docs: https://about.chiperka.com

---

## Principles

- **The CLI is free and Apache-2.0. Forever.** No open-core trick, no "basic features migrate to a paid tier."
- **Your `.chiperka` files live in your repo.** Not in our database. Not in a lock-in format. If you stop using Chiperka, the files are still useful documentation.
- **Your code is your code.** Chiperka never trains models on your data.
- **MCP-first, not MCP-only.** When other agent protocols ship with real adoption, we'll write adapters. No panic.
- **Local first.** Everything runs on your machine, against your Docker, on your network.

---

## Contributing

Issues, PRs, and `.chiperka` files for real-world projects are all welcome. If you're writing scenarios for a popular open-source backend, open a PR against [`chiperka/chiperka-examples`](https://github.com/chiperka/chiperka-examples) — it helps everyone.

## Links

- [Documentation](https://about.chiperka.com)
- [`.chiperka` file specification](https://github.com/chiperka/specification)
- [JetBrains plugin](https://plugins.jetbrains.com/plugin/30418-chiperka)
- [Issues](https://github.com/chiperka/chiperka/issues)
