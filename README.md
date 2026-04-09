# Chiperka

Declarative API & integration test runner. Write YAML, run against real services in Docker. No SDKs, no glue code.

```yaml
name: MyTest

tests:
  - name: Service responds with 200
    services:
      - name: webserver
        image: nginx:alpine
        healthcheck:
          test: "wget -q --spider http://localhost:80/"
          retries: 30
    execution:
      executor: http
      target: http://webserver
      request:
        method: GET
        url: /
    assertions:
      - response:
          statusCode: 200
```

```
$ chiperka run ./tests

Chiperka Test Runner v1.5.0
  1 tests in 1 suites, 8 workers

Running tests
  ✓ [100%] MyTest/Service responds with 200 (2.484s)

PASSED 1/1 in 2.586s
```

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
  finie/chiperka:latest chiperka run ./tests
```

Requires Docker for local use.

## Features

- **Just YAML** — define services, HTTP requests, CLI commands, and assertions
- **Full isolation** — every test gets its own Docker network
- **Parallel by default** — tests run concurrently out of the box
- **HTML & JUnit reports** — `--html report.html` / `--junit report.xml`
- **Snapshot testing** — compare responses against saved snapshots
- **Cloud mode** — run tests remotely with `--cloud`, no local Docker needed ([create account](https://chiperka.com))

## Quick start

```bash
# Create a test
chiperka init

# Run it
chiperka run ./tests

# Generate HTML report
chiperka run ./tests --html report.html
```

## Links

- [Documentation](https://about.chiperka.com/getting-started)
- [Website](https://about.chiperka.com)
- [Cloud App](https://chiperka.com)
- [JetBrains Plugin](https://plugins.jetbrains.com/plugin/30418-chiperka-test-runner)
