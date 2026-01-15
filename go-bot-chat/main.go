package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/langchain-ai/langsmith-go/examples/otel_anthropic/traceanthropic"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Validate keys
	langsmithKey := os.Getenv("LANGSMITH_API_KEY")
	if langsmithKey == "" {
		log.Fatal("LANGSMITH_API_KEY is required")
	}

	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	projectName := os.Getenv("LANGSMITH_PROJECT")
	if projectName == "" {
		projectName = "go-bot-chat"
	}

	// Initialize OpenTelemetry tracing to LangSmith
	shutdown, err := initTracer(langsmithKey, projectName)
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer shutdown()

	// Create Anthropic client with automatic tracing
	client := anthropic.NewClient(
		option.WithAPIKey(anthropicKey),
		option.WithHTTPClient(traceanthropic.Client()),
	)

	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)
	tracer := otel.Tracer("go-chat-demo")

	// Generate a unique thread ID per session
	threadID := uuid.New().String()

	// Maintain conversation history 
	var conversationHistory []anthropic.MessageParam

	fmt.Printf("Chat with Claude (tracing to LangSmith project: %s)\n", projectName)
	fmt.Printf("Thread ID: %s\n", threadID)
	fmt.Println("Type 'quit' to exit.\n")

	for {
		fmt.Print("You: ")
		userMessage, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error reading input: %v", err)
			continue
		}

		userMessage = strings.TrimSpace(userMessage)
		if userMessage == "" {
			continue
		}
		if strings.ToLower(userMessage) == "quit" {
			fmt.Println("\nFlushing traces to LangSmith...")
			if tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
				if err := tp.ForceFlush(ctx); err != nil {
					log.Printf("Error flushing traces: %v", err)
				}
			}
			fmt.Println("Goodbye!")
			return
		}

		// Add user message to history
		conversationHistory = append(conversationHistory,
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
		)

		// Create a parent span for this conversation turn with thread metadata
		// This groups all turns with the same session_id into a thread in LangSmith
		turnCtx, turnSpan := tracer.Start(ctx, "chat_turn",
			trace.WithAttributes(
				attribute.String("langsmith.trace.name", "go-bot"),
				attribute.String("langsmith.metadata.session_id", threadID),
				attribute.String("langsmith.span.kind", "chain"),
				// Set input on the parent span for Thread view
				attribute.String("gen_ai.prompt", userMessage),
			),
		)

		resp, err := client.Messages.New(turnCtx, anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-sonnet-4-20250514"),
			MaxTokens: 1024,
			Messages:  conversationHistory,
		})

		if err != nil {
			log.Printf("Error: %v\n", err)
			turnSpan.End()
			continue
		}

		// Extract and display response (concat all text blocks)
		var textParts []string
		for _, block := range resp.Content {
			if block.Type == "text" {
				textParts = append(textParts, block.Text)
			}
		}
		responseText := strings.Join(textParts, "\n")

		turnSpan.SetAttributes(
			attribute.String("gen_ai.completion", responseText),
			attribute.Int64("gen_ai.usage.input_tokens", resp.Usage.InputTokens),
			attribute.Int64("gen_ai.usage.output_tokens", resp.Usage.OutputTokens),
		)

		// Add assistant response to history
		conversationHistory = append(conversationHistory,
			anthropic.NewAssistantMessage(anthropic.NewTextBlock(responseText)),
		)

		turnSpan.End()

		fmt.Printf("\nClaude: %s\n\n", responseText)
	}
}

func initTracer(apiKey, projectName string) (func(), error) {
	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("go-chat-demo"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint("api.smith.langchain.com"),
		otlptracehttp.WithURLPath("/otel/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"x-api-key":         apiKey,
			"Langsmith-Project": projectName,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("creating exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Second)),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func() {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error shutting down tracer: %v", err)
		}
	}, nil
}
