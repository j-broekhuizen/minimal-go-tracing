# Go Tracing with LangSmith

A demo Go application showing how to trace LLM calls to [LangSmith](https://smith.langchain.com) using the [LangSmith Go SDK](https://github.com/langchain-ai/langsmith-go).

## Features

- Multi-turn chat with Claude (conversation history preserved)
- Automatic tracing via the `langsmith-go` SDK
- Thread support for grouping conversation turns in LangSmith
- Token usage tracking visible in LangSmith UI

## Prerequisites

- Go 1.23+
- LangSmith API key
- Anthropic API key

## Setup

1. Clone this repo

2. Create a `.env` file from the example:
```bash
cp .env.example .env
```

3. Fill in your API keys in `.env`:
```bash
LANGSMITH_API_KEY=lsv2_pt_xxxxx
LANGSMITH_PROJECT=go-tracing-demo
ANTHROPIC_API_KEY=sk-ant-xxxxx
```

4. Install dependencies:
```bash
go mod tidy
```

## Run

```bash
go run ./app
```

You'll see:
```
Chat with Claude (tracing to LangSmith project: go-tracing-demo)
Thread ID: f47ac10b-58cc-4372-a567-0e02b2c3d479
Type 'quit' to exit.

You: Hello!
Claude: Hello! How can I help you today?
(15 input, 12 output tokens)

You: quit
Flushing traces to LangSmith...
Goodbye!
```

## View Traces

1. Go to [smith.langchain.com](https://smith.langchain.com)
2. Navigate to your project
3. Click **Runs** to see individual traces
4. Click **Threads** to see grouped conversations

## How It Works

### Architecture

```
Your App
    │
    ├── Parent span (go-bot) ─────────────────────┐
    │   ├── langsmith.metadata.session_id         │ Thread grouping
    │   ├── gen_ai.prompt                         │ Input for UI
    │   ├── gen_ai.completion                     │ Output for UI
    │   └── gen_ai.usage.*                        │ Token counts
    │                                             │
    └── traceanthropic wrapper ───────────────────┤
        └── Child span (anthropic.messages)       │ Auto-traced LLM call
            ├── gen_ai.system = "anthropic"       │
            ├── gen_ai.request.model              │
            └── gen_ai.usage.*                    │
                                                  │
                        ▼                         │
              OpenTelemetry SDK                   │
              (BatchSpanProcessor)                │
                        │                         │
                        ▼                         │
                   LangSmith                      │
              (OTLP /otel/v1/traces)              │
```

### Key Files

| File            | Purpose                       |
| --------------- | ----------------------------- |
| `app/main.go`   | Chat application with tracing |
| `langsmith-go/` | The LangSmith Go SDK          |
| `.env`          | API keys and configuration    |

### Key Concepts

- **Thread ID**: A UUID generated per session, passed as `langsmith.metadata.session_id` to group traces
- **Parent Span**: Created manually for each turn, contains thread metadata and I/O for the Thread view
- **Child Span**: Auto-created by `traceanthropic` wrapper for each Anthropic API call
- **Context Propagation**: Passing `turnCtx` to the API call links the child span to the parent

## Environment Variables

| Variable            | Required | Description                       |
| ------------------- | -------- | --------------------------------- |
| `LANGSMITH_API_KEY` | Yes      | Your LangSmith API key            |
| `LANGSMITH_PROJECT` | No       | Project name (default: "default") |
| `ANTHROPIC_API_KEY` | Yes      | Your Anthropic API key            |

## Resources

- [LangSmith Docs](https://docs.smith.langchain.com)
- [LangSmith OTel Tracing](https://docs.smith.langchain.com/observability/how_to_guides/trace_with_opentelemetry)
- [LangSmith Threads](https://docs.smith.langchain.com/observability/how_to_guides/threads)
- [langsmith-go SDK](https://github.com/langchain-ai/langsmith-go)
