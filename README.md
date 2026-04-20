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

Every `.chiperka` file has a `kind:` field that declares what it is. There are three kinds:

### Endpoint — what your service exposes

```yaml
kind: endpoint
name: login
service: api
method: POST
url: /auth/login
inputs:
  - name: email
    type: string
    required: true
  - name: password
    type: string
    required: true
```

Endpoints declare *what can be called* — the service, method, URL, and expected inputs. They carry no concrete data. Think of them as the contract.

### Service — how to run it

```yaml
kind: service
name: api
image: ghcr.io/myorg/api:latest
environment:
  DB_URL: postgres://db:5432/test
healthcheck:
  test: "curl -f http://localhost:8080/health"
  retries: 30
```

Service templates are reusable Docker service definitions. Tests reference them with `ref:`.

### Test — a concrete invocation with assertions

```yaml
kind: test
name: auth
tests:
  - name: login-with-valid-credentials
    tags: [smoke, auth]
    services:
      - ref: api
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

A real project usually has many such files — organized by area. The whole corpus is your runnable specification. Chiperka discovers them automatically by walking the repo.

You can also drop a small `.chiperka/chiperka.yaml` in the repo root for **CLI configuration** — discovery paths, report configuration, and similar cross-run settings. It's optional and does not declare any project resources.

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
- **`chiperka_list(kind)`** — list all endpoints, tests, or services in the project
- **`chiperka_get(kind, name)`** — read full detail of a single resource
- **`chiperka_validate`** — validate files without running them
- **`chiperka_execute`** — run an inline scenario (great for "what if I send this?" experiments)
- **`chiperka_run`** — execute tests end-to-end and return structured results
- **`chiperka_read_runs/run/test/artifact`** — drill into stored results progressively
- **`chiperka_report_*`** — generate and read reports (HTML, JUnit, custom)

The agent workflow: list endpoints to see what exists, compare with tests to find coverage gaps, write new tests, run them, analyze results, and iterate — all autonomously.

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
1. Your team writes .chiperka files: endpoints (what the service exposes),
   services (how to run them), and tests (concrete invocations with assertions).
2. Anyone who clones the repo runs `chiperka mcp`.
3. Their AI agent connects via MCP, discovers all endpoints, services, and tests.
4. The agent lists endpoints, finds ones without tests, writes tests,
   runs them against real Docker services, and iterates until coverage is complete.
5. Results are facts from real runs, not guesses from static code reading.
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
- **Weight-based scheduling.** Each service has a weight (default 1). Tests run in parallel as long as total weight fits within the configured capacity. Capacity is set in `~/.chiperka/config.json` or via `--capacity` flag.

---

## Contributing

Issues, PRs, and `.chiperka` files for real-world projects are all welcome. If you're writing scenarios for a popular open-source backend, open a PR against [`chiperka/chiperka-examples`](https://github.com/chiperka/chiperka-examples) — it helps everyone.

## Links

- [Documentation](https://about.chiperka.com)
- [`.chiperka` file specification](https://github.com/chiperka/specification)
- [JetBrains plugin](https://plugins.jetbrains.com/plugin/30418-chiperka)
- [Issues](https://github.com/chiperka/chiperka/issues)
