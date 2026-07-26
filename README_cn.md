# setup-localai

setup-localai 是一个轻量的本地配置辅助工具，目标是把本地运行的 LLM 服务（例如通过 LocalAI 对外暴露的 OpenAI 兼容接口）自动配置给常用的 CLI 工具，如 Codex 和 Claude Code。

它会根据内置模板生成客户端配置，并自动下载/安装对应的可执行文件到用户目录下，减少手动配置的成本。

## 功能特点

- 自动生成 Codex 配置文件：~/.codex/config.toml
- 自动生成 Claude Code 配置文件：~/.claude.json
- 默认不会下载并安装 Codex / Claude Code 二进制文件；如需安装，可显式传入 --install-codex 和/或 --install-claude
- 支持 Linux / macOS / Windows
- 内置默认配置，开箱即用地对接本地兼容 OpenAI 的服务

## 适用场景

如果你已经在本地启动了一个兼容 OpenAI API 的服务，例如：

- LocalAI
- Kimi 本地服务
- 其他兼容 /v1/chat/completions 或 /v1/responses 的接口服务

那么这个项目可以帮助你快速把它接到 Codex 和 Claude Code 这类工具上。

## 项目结构

- [main.go](main.go)：主程序，负责读取模板、生成配置、下载安装二进制文件
- [assets/vars.json](assets/vars.json)：默认 API 配置、模型名称、下载地址和校验信息
- [assets/codex_config.toml.tmpl](assets/codex_config.toml.tmpl)：Codex 配置模板
- [assets/claude_json.tmpl](assets/claude_json.tmpl)：Claude Code 配置模板

## 前置条件

在使用前请确保：

1. 本地服务已经启动并可访问
2. 服务提供兼容 OpenAI 的 API 接口
3. 你已经确认好：
   - API Key
   - Base URL
   - 模型名称

默认配置中使用的是：

- Base URL: http://127.0.0.1:8080/v1
- Model: kimi-k3
- API Key: sk-local-kimi-k3-REPLACE-AT-BUILD

如果你的环境不同，请先修改 [assets/vars.json](assets/vars.json) 中对应字段。

## 快速开始

### 1. 构建程序

在项目根目录执行：

```bash
go build -o setuplocalai .
```

### 2. 运行程序

```bash
./setuplocalai
```

程序会完成以下操作：

- 写入 Codex 配置到 ~/.codex/config.toml
- 写入 Claude 配置到 ~/.claude.json
- 默认不安装 Codex / Claude 可执行文件；如需安装可使用 --install-codex 和/或 --install-claude

### 3. 将安装目录加入 PATH

如果你的 shell 里还没有包含该目录，需要手动添加：

```bash
export PATH="$HOME/.kimi-toolkit/bin:$PATH"
```

你也可以把这行命令加入 shell 配置文件（如 ~/.bashrc、~/.zshrc）。

## 配置说明

### 修改默认配置

程序现在会优先读取外部 vars.json 文件作为最高优先级。它会先检查 SETUP_LOCALAI_VARS_JSON 环境变量指定的路径，然后查找当前工作目录下的 vars.json，最后回退到内置的 [assets/vars.json](assets/vars.json) 默认值。

不需要重新构建即可通过在可执行文件旁边放置 vars.json，或者设置 SETUP_LOCALAI_VARS_JSON 来覆盖配置：

```json
{
  "api_key": "你的密钥",
  "base_url": "http://127.0.0.1:8080/v1",
  "model": "你的模型名"
}
```

### 发布自动化

仓库包含一个 GitHub Actions 工作流，会构建 6 个系统平台的产物，并在推送 v* 标签时自动上传到 GitHub Releases。

### 一键发布脚本

使用 `release.sh` 来创建注释标签并将当前分支与标签推送到 GitHub。

```bash
./release.sh 1.0.0
```

这将创建并推送 `v1.0.0`，触发发布工作流并上传已构建的二进制文件。

### 生成内容

程序会生成以下文件：

- ~/.codex/config.toml
- ~/.claude.json
- ~/.kimi-toolkit/bin/ 下的可执行文件和对应的内容寻址文件

## 注意事项

- 当前脚本会在已有配置文件存在且内容发生变化时进行备份，备份文件会保留原内容的哈希后缀。
- 下载的二进制会被安装到内容寻址路径，并通过符号链接或复制文件提供一个统一的入口。
- 如果你使用的是 Windows，请注意程序会自动补上 .exe 后缀。

## 贡献与扩展

如果你想扩展这个项目，可以考虑：

- 增加更多客户端支持（如 OpenAI CLI、Aider 等）
- 支持从环境变量读取配置，而不是写死在模板里
- 增加更灵活的版本和镜像源配置
