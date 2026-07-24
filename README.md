# setup-localai

setup-localai is a lightweight local configuration helper designed to automatically configure common CLI tools such as Codex and Claude Code to work with a locally running LLM service that exposes an OpenAI-compatible API.

It generates client-side configuration from built-in templates and can optionally download and install the corresponding executables into the user directory, reducing manual setup effort.

> The Chinese version of this document is available at [README_cn.md](README_cn.md).

## Features

- Generate Codex configuration at ~/.codex/config.toml
- Generate Claude Code configuration at ~/.claude.json
- By default, Codex and Claude binaries are not installed automatically; enable them explicitly with --install-codex and/or --install-claude
- Support Linux / macOS / Windows
- Built-in default configuration for connecting to locally exposed OpenAI-compatible services

## Use Cases

If you already have a local service exposing an OpenAI-compatible API, such as:

- LocalAI
- Kimi local service
- Any service compatible with /v1/chat/completions or /v1/responses

this project can help you quickly connect it to tools such as Codex and Claude Code.

## Project Structure

- [main.go](main.go): main program that loads templates, generates config, and installs binaries
- [assets/vars.json](assets/vars.json): default API settings, model name, download URLs, and checksum information
- [assets/codex_config.toml.tmpl](assets/codex_config.toml.tmpl): Codex configuration template
- [assets/claude_json.tmpl](assets/claude_json.tmpl): Claude Code configuration template

## Prerequisites

Before using it, make sure:

1. Your local service is already running and reachable
2. The service exposes an OpenAI-compatible API
3. You have confirmed:
   - API key
   - Base URL
   - Model name

The default configuration uses:

- Base URL: http://127.0.0.1:8080/v1
- Model: kimi-k3
- API Key: sk-local-kimi-k3-REPLACE-AT-BUILD

If your environment differs, update the corresponding fields in [assets/vars.json](assets/vars.json).

## Quick Start

### 1. Build the program

Run this in the project root:

```bash
go build -o setuplocalai .
```

### 2. Run the program

```bash
./setuplocalai
```

The program will:

- Write the Codex config to ~/.codex/config.toml
- Write the Claude config to ~/.claude.json
- Skip installing Codex / Claude binaries by default; use --install-codex and/or --install-claude if you want them installed

### 3. Add the install directory to PATH

If your shell does not already include the directory, add:

```bash
export PATH="$HOME/.kimi-toolkit/bin:$PATH"
```

You can also place this line in your shell config file such as ~/.bashrc or ~/.zshrc.

## Configuration Notes

### Customize default settings

Edit [assets/vars.json](assets/vars.json), rebuild, and then run the program again:

```json
{
  "api_key": "your key",
  "base_url": "http://127.0.0.1:8080/v1",
  "model": "your model"
}
```

### Generated files

The program will generate:

- ~/.codex/config.toml
- ~/.claude.json
- Executables and content-addressed files under ~/.kimi-toolkit/bin/

## Notes

- When existing config files are present and their contents change, the script creates a backup with a hash suffix.
- Downloaded binaries are installed into content-addressed paths and exposed through a symlink or copied file for a unified entry point.
- On Windows, the program will automatically append the .exe suffix.

## Contributing and Extending

If you want to extend the project, consider:

- Adding support for more clients (for example, OpenAI CLI or Aider)
- Reading configuration from environment variables instead of hardcoding it in templates
- Adding more flexible version and mirror-source configuration
