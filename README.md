# Chiperka — Declarative API & Integration Test Runner

[Chiperka](https://about.chiperka.com/) is a YAML-based test runner for APIs and integrations. Define services, HTTP requests, and assertions in `.chiperka` files — Chiperka handles Docker containers, health checks, execution, and cleanup.

## CLI

- **Just YAML** — no SDKs, no test frameworks, no glue code
- **Full isolation** — every test gets its own Docker network
- **Parallel by default** — tests run concurrently out of the box
- **HTML & JUnit reports** — `--html report.html` / `--junit report.xml`
- **Cloud mode** — run tests on a remote server with `--cloud`
- **Snapshot testing** — compare responses against saved snapshots

Docker is required for local use. Cloud mode (`--cloud`) runs tests on a remote server and does not require Docker locally.

**[Getting started](https://about.chiperka.com/getting-started)** · **[About](https://about.chiperka.com/)**

## JetBrains Plugin

Run `.chiperka` tests directly from PhpStorm, IntelliJ IDEA, WebStorm, and other JetBrains IDEs — gutter play buttons, Test Runner integration, and schema validation.

**[Install from JetBrains Marketplace](https://plugins.jetbrains.com/plugin/30418-chiperka-test-runner)** · **[Plugin documentation](https://about.chiperka.com/ide-plugin)**
