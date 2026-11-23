# AI-Auto-Test

[简体中文](README.zh-CN.md) · [Downloads](https://github.com/DamonZS/AI-Auto-Test/releases/latest) · [MIT License](LICENSE)

AI-Auto-Test is an AI-assisted QA workbench for teams. It links requirements, versions, environments, test data, cases, runs, and defects in one workspace. Run it as a Windows or macOS desktop app, or deploy the web workbench and workers on Linux.

## What you can do

- Import requirements from PDF, Word (`.docx`), Excel (`.xlsx`), CSV, Markdown, or text. Turn them into reviewable functional points and acceptance criteria.
- Generate and review manual, UI, HTTP API, and performance cases. Record browser actions on desktop, replay structured UI cases with Playwright, and group approved cases into regression suites. Defensive security checklists are verified manually.
- Generate synthetic test data, bind a dataset to a case, track release coverage and defects, and review step evidence and reports.
- Recover eligible UI runs from locator drift. AI can propose a replacement accessible name for a missing control. Passing the original assertions produces a candidate patch for owner approval. Approval saves a new case revision.
- Connect coding agents through MCP over Streamable HTTP. A workspace token scopes access. Generating cases from submitted code requires a client that supports MCP Sampling, and generated drafts go through human review.

## Install

| Platform | Package |
| --- | --- |
| Windows x64 | [`.exe` installer](https://github.com/DamonZS/AI-Auto-Test/releases/latest/download/Auto-Test-windows-x64-setup.exe) |
| macOS Apple Silicon | [`.dmg` disk image](https://github.com/DamonZS/AI-Auto-Test/releases/latest/download/Auto-Test-macos-arm64.dmg) |
| Linux amd64 | [Docker image archive](https://github.com/DamonZS/AI-Auto-Test/releases/latest/download/Auto-Test-linux-amd64.tar.gz) |

Open the desktop app and create a workspace and owner account. In Settings, activate the Desktop Worker for the workspace. Configure an OpenAI-compatible model in Settings to enable AI generation and locator recovery. For Docker Compose setup, local builds, data paths, and upgrade instructions, see the [deployment guide](docs/deployment/README.md).

## Architecture and documentation

The Vue workbench talks to a Go control plane backed by SQLite. Python workers run browser, API, and performance tests. Playwright is the baseline browser driver. Deep Agents orchestrates natural-language UI execution. The desktop build embeds the workbench in a Qt window.

- [Workbench workflow](docs/workbench-redesign.md)
- [MCP setup](docs/api/mcp.md) and [Sampling protocol](docs/api/mcp-sampling.md)
- [UI locator recovery and approval](docs/api/ui-healing.md)
- [Deployment and source builds](docs/deployment/README.md)

## License

Copyright (c) 2026 DamonZS. Project source code is available under the [MIT License](LICENSE). Third-party components retain their own licenses and attributions. See the [copyright notice](COPYRIGHT.md).
