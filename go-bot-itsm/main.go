package main

import (
	"bufio"
	"context"
	"encoding/json"
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

// AccessRequest is a minimal ticket object for an ITSM access request.
type AccessRequest struct {
	ID                 string `json:"id"`
	Type               string `json:"type"` // "access_request"
	RequestedFor       string `json:"requested_for"`
	Resource           string `json:"resource"`
	AccessLevel        string `json:"access_level"`
	Duration           string `json:"duration"`
	BusinessJustif     string `json:"business_justification"`
	ApprovalsRequired  string `json:"approvals_required"`
	RiskLevel          string `json:"risk_level"`
	Status             string `json:"status"`
	CreatedAt          string `json:"created_at"`
	RecommendedActions string `json:"recommended_actions"`
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

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
		projectName = "go-bot-itsm"
	}

	// Initialize OTEL tracing to LangSmith
	shutdown, err := initTracer(langsmithKey, projectName)
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer shutdown()

	client := anthropic.NewClient(
		option.WithAPIKey(anthropicKey),
		option.WithHTTPClient(traceanthropic.Client()),
	)

	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)
	tracer := otel.Tracer("go-bot-itsm")

	threadID := uuid.New().String()

	systemPrompt := `You are an ITSM assistant. Your job is to help users create ACCESS REQUEST tickets.
		Be concise, practical, and enterprise-friendly.

		When user asks for access, respond in this format:

		1) Quick classification: "Request Type: Access Request"
		2) Ask at most 2 clarifying questions if needed (duration, justification, access level, resource)
		3) When enough info exists, produce:
		- "Ticket Draft" with short structured fields
		- "Approvals" required
		- "Next Steps"
		Keep it friendly and efficient.`

	// Conversation history
	var conversationHistory []anthropic.MessageParam

	fmt.Printf("go-bot-itsm (tracing to LangSmith project: %s)\n", projectName)
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

		// Add user input to history
		conversationHistory = append(conversationHistory,
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
		)

		// Span per turn (threaded via session_id)
		turnCtx, turnSpan := tracer.Start(ctx, "itsm_turn",
			trace.WithAttributes(
				attribute.String("langsmith.trace.name", "go-bot-itsm"),
				attribute.String("langsmith.metadata.session_id", threadID),
				attribute.String("langsmith.span.kind", "chain"),
				attribute.String("gen_ai.prompt", userMessage),
				attribute.String("itsm.category", "access_request_demo"),
			),
		)

		resp, err := client.Messages.New(turnCtx, anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-sonnet-4-20250514"),
			MaxTokens: 1024,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: conversationHistory,
		})

		if err != nil {
			log.Printf("Error: %v\n", err)
			turnSpan.End()
			continue
		}

		// Extract model response text
		var textParts []string
		for _, block := range resp.Content {
			if block.Type == "text" {
				textParts = append(textParts, block.Text)
			}
		}
		responseText := strings.Join(textParts, "\n")

		ticketDraft := inferAccessRequestDraft(userMessage)
		ticketJSON, _ := json.MarshalIndent(ticketDraft, "", "  ")

		turnSpan.SetAttributes(
			attribute.String("gen_ai.completion", responseText),
			attribute.Int64("gen_ai.usage.input_tokens", resp.Usage.InputTokens),
			attribute.Int64("gen_ai.usage.output_tokens", resp.Usage.OutputTokens),
			attribute.String("itsm.ticket_draft_json", string(ticketJSON)),
		)

		// Add assistant response to history
		conversationHistory = append(conversationHistory,
			anthropic.NewAssistantMessage(anthropic.NewTextBlock(responseText)),
		)

		turnSpan.End()

		fmt.Printf("\nITSM Assistant: %s\n\n", responseText)
	}
}

// inferAccessRequestDraft creates a small, local ticket draft object
// This is intentionally simple and does not need perfect extraction.
func inferAccessRequestDraft(userMessage string) AccessRequest {
	now := time.Now().UTC().Format(time.RFC3339)
	id := "AR-" + strings.ToUpper(uuid.New().String()[:8])

	resource := "unknown"
	accessLevel := "unknown"
	duration := "unknown"

	lower := strings.ToLower(userMessage)

	// extremely lightweight heuristics
	if strings.Contains(lower, "snowflake") {
		resource = "snowflake"
	}
	if strings.Contains(lower, "datadog") {
		resource = "datadog"
	}
	if strings.Contains(lower, "github") {
		resource = "github"
	}
	if strings.Contains(lower, "prod") || strings.Contains(lower, "production") {
		resource = resource + "_prod"
	}

	if strings.Contains(lower, "admin") {
		accessLevel = "admin"
	} else if strings.Contains(lower, "read") {
		accessLevel = "read"
	} else if strings.Contains(lower, "write") {
		accessLevel = "write"
	}

	if strings.Contains(lower, "24") && strings.Contains(lower, "hour") {
		duration = "24h"
	} else if strings.Contains(lower, "7") && strings.Contains(lower, "day") {
		duration = "7d"
	}

	risk := "medium"
	if strings.Contains(lower, "prod") || strings.Contains(lower, "admin") {
		risk = "high"
	}

	return AccessRequest{
		ID:                 id,
		Type:               "access_request",
		RequestedFor:       "self",
		Resource:           resource,
		AccessLevel:        accessLevel,
		Duration:           duration,
		BusinessJustif:     "provided_in_chat",
		ApprovalsRequired:  "manager + system_owner",
		RiskLevel:          risk,
		Status:             "draft",
		CreatedAt:          now,
		RecommendedActions: "collect justification; confirm duration; route for approval; provision access; log audit",
	}
}

func initTracer(apiKey, projectName string) (func(), error) {
	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("go-bot-itsm"),
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
