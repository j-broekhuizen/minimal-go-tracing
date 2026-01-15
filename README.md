# Go Tracing with LangSmith

Basic Go apps showing how to trace to [LangSmith](https://smith.langchain.com) using the [LangSmith Go SDK](https://github.com/langchain-ai/langsmith-go).

## Apps

| App           | Description                  |
| ------------- | ---------------------------- |
| `go-bot-chat` | Basic multi-turn chat        |
| `go-bot-itsm` | ITSM access request workflow |

## Features

- Multi-turn chat (conversation history preserved)
- Tracing via the `langsmith-go` SDK
- Thread support for grouping conversation turns in LangSmith

## Prereqs

- Go 1.23+
- LangSmith API key
- Anthropic API key

## Setup

1. Clone this repo

2. Create a `.env` file from the example:
```bash
cp .env.example .env
```

3. Add API keys in `.env`:
```bash
LANGSMITH_API_KEY=lsv2_pt_xxxxx
ANTHROPIC_API_KEY=sk-ant-xxxxx
```

4. Install deps:
```bash
go mod tidy
```

## Run

### go-bot-chat 

```bash
go run ./go-bot-chat
```

```
Chat with Claude (tracing to LangSmith project: go-bot-chat)
Thread ID: f47ac10b-58cc-4372-a567-0e02b2c3d479
Type 'quit' to exit.

You: Hello!
Claude: Hello! How can I help you today?
```

### go-bot-itsm 

```bash
go run ./go-bot-itsm
```

```
go-bot-itsm (tracing to LangSmith project: go-bot-itsm)
Thread ID: a1b2c3d4-5678-90ab-cdef-1234567890ab
Type 'quit' to exit.

You: I need access to Datadog admin for on-call this week
ITSM Assistant: Request Type: Access Request

I can help you with that! To create your access request, I need a couple of details:
1. What's the duration needed? (e.g., 7 days)
2. Brief business justification?
```

The ITSM demo traces include additional metadata:
- `itsm.category`: Type of ITSM request
- `itsm.ticket_draft_json`: Generated ticket draft object

## View Traces

1. Go to [smith.langchain.com](https://smith.langchain.com)
2. Navigate to your project
3. Click **Runs** to see individual traces
4. Click **Threads** to see grouped conversations


## Env Vars

| Variable            | Required | Description                                          |
| ------------------- | -------- | ---------------------------------------------------- |
| `LANGSMITH_API_KEY` | Yes      | Your LangSmith API key                               |
| `LANGSMITH_PROJECT` | No       | Override project name (each app has its own default) |
| `ANTHROPIC_API_KEY` | Yes      | Your Anthropic API key                               |

**Default projects:**
- `go-bot-chat` → traces to `go-bot-chat` project
- `go-bot-itsm` → traces to `go-bot-itsm` project

## Resources

- [LangSmith Docs](https://docs.smith.langchain.com)
- [LangSmith OTel Tracing](https://docs.smith.langchain.com/observability/how_to_guides/trace_with_opentelemetry)
- [LangSmith Threads](https://docs.smith.langchain.com/observability/how_to_guides/threads)
- [langsmith-go SDK](https://github.com/langchain-ai/langsmith-go)
