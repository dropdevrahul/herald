package agents

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/dropdevrahul/herald/src/memory"
	"github.com/dropdevrahul/herald/src/model"
	"github.com/dropdevrahul/herald/src/worklows"
)

// StopFunc lets a caller end an agent run early. It is evaluated after each
// turn that issued tool calls, receiving the 1-based turn number and the most
// recent model content; returning true stops the loop.
type StopFunc func(turn int, lastContent string) bool

// ApprovalDecision is returned by a ToolApprover to indicate whether a tool
// call should proceed, be denied, or have its arguments rewritten.
type ApprovalDecision struct {
	Approved bool
	Reason   string
	Args     string
}

// ToolApprover is an optional gate consulted before each tool execution.
// Return an error to abort the run; return ApprovalDecision{Approved:false}
// to deny the call (the denial is threaded back to the model as a tool
// result); return ApprovalDecision{Approved:true, Args:"..."} to rewrite
// the arguments before execution.
type ToolApprover func(ctx context.Context, call model.ToolCall) (ApprovalDecision, error)

// Event is a lifecycle event emitted by the Agent loop to registered Hooks.
type Event struct {
	Type    string
	Turn    int
	Tool    string
	Args    string
	Result  string
	Content string
}

// Hook observes Agent lifecycle Events. Hooks are invoked synchronously and
// must not mutate the run.
type Hook func(ctx context.Context, ev Event)

// AgentConfig configures a generic Agent. Only Tools and a SystemPrompt are
// typically needed; MaxTurns and Temperature have sensible defaults.
type AgentConfig struct {
	SystemPrompt string
	Tools        []workflows.Tool
	MaxTurns     int
	Temperature  float64
	Stop         StopFunc
	Memory       memory.Memory
	Approver     ToolApprover
	Hooks        []Hook
}

// Agent is a generic, provider-agnostic agent runtime: it drives a multi-turn
// tool-calling loop against any model.Model until the model stops requesting
// tools, a stop condition fires, or the turn budget is exhausted.
type Agent struct {
	model model.Model
	cfg   AgentConfig
}

func NewAgent(m model.Model, cfg AgentConfig) *Agent {
	return &Agent{model: m, cfg: cfg}
}

// emit dispatches an Event to all configured Hooks. It is a no-op when no
// hooks are set.
func (a *Agent) emit(ctx context.Context, ev Event) {
	for _, h := range a.cfg.Hooks {
		if h != nil {
			h(ctx, ev)
		}
	}
}

// toModelTools converts tools into the model.Tools shape, including each tool's
// JSON-schema Parameters so the model knows how to call them.
func toModelTools(tools []workflows.Tool) model.Tools {
	var result model.Tools
	for _, t := range tools {
		result = append(result, model.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return result
}

func (a *Agent) modelOptions() *model.ModelOptions {
	temp := a.cfg.Temperature
	if temp == 0 {
		temp = 0.7
	}
	opts := &model.ModelOptions{Temperature: temp}
	if len(a.cfg.Tools) > 0 {
		opts.Tools = toModelTools(a.cfg.Tools)
		opts.ToolChoice = model.ToolChoiceAuto
	}
	return opts
}

func (a *Agent) maxTurns() int {
	if a.cfg.MaxTurns <= 0 {
		return 5
	}
	return a.cfg.MaxTurns
}

func (a *Agent) callTool(ctx context.Context, tc model.ToolCall) string {
	for _, tool := range a.cfg.Tools {
		if tc.Function.Name == tool.Name() {
			args := tc.Function.Arguments
			if args == "" {
				args = "{}"
			}
			result, err := tool.Call(ctx, args)
			if err != nil {
				return "Error: " + err.Error()
			}
			if result == "" {
				return "Tool executed"
			}
			return result
		}
	}
	return "Tool not found: " + tc.Function.Name
}

// runLoop is the shared multi-turn engine. handler may be nil; when set it is
// invoked with ("content", delta) for streamed text and ("tool", "[name] result")
// after each tool call.
func (a *Agent) runLoop(ctx context.Context, input string, handler func(node string, result string) error) (string, error) {
	var messages []model.Message
	if a.cfg.SystemPrompt != "" {
		messages = append(messages, model.Message{Role: model.RoleSystem, Content: a.cfg.SystemPrompt})
	}
	if a.cfg.Memory != nil {
		messages = append(messages, a.cfg.Memory.Messages()...)
	}
	userMsg := model.Message{Role: model.RoleUser, Content: input}
	messages = append(messages, userMsg)
	if a.cfg.Memory != nil {
		a.cfg.Memory.Add(userMsg)
	}

	opts := a.modelOptions()
	maxTurns := a.maxTurns()

	lastContent := ""
	for turn := 0; turn < maxTurns; turn++ {
		a.emit(ctx, Event{Type: "turn_start", Turn: turn + 1})
		resultChan := a.model.Stream(ctx, messages, opts)

		var content string
		var toolCalls []model.ToolCall

		for result := range resultChan {
			if result.Err != nil {
				return "", result.Err
			}
			if result.Delta != "" {
				content += result.Delta
				if handler != nil {
					handler("content", result.Delta)
				}
			}
			if result.Content != "" && result.Delta == "" {
				content += result.Content
				if handler != nil {
					handler("content", result.Content)
				}
			}
			if len(result.ToolCalls) > 0 {
				toolCalls = result.ToolCalls
			}
		}

		lastContent = content

		a.emit(ctx, Event{Type: "model_response", Turn: turn + 1, Content: content})

		// No tool calls: the model produced a final answer.
		if len(toolCalls) == 0 {
			if a.cfg.Memory != nil {
				a.cfg.Memory.Add(model.Message{Role: model.RoleAssistant, Content: content})
			}
			a.emit(ctx, Event{Type: "finish", Content: content})
			return content, nil
		}

		assistantMsg := model.Message{
			Role:      model.RoleAssistant,
			Content:   content,
			ToolCalls: toolCalls,
		}
		messages = append(messages, assistantMsg)
		if a.cfg.Memory != nil {
			a.cfg.Memory.Add(assistantMsg)
		}

		for _, tc := range toolCalls {
			a.emit(ctx, Event{Type: "tool_start", Turn: turn + 1, Tool: tc.Function.Name, Args: tc.Function.Arguments})
			var toolResult string
			if a.cfg.Approver != nil {
				decision, err := a.cfg.Approver(ctx, tc)
				if err != nil {
					return "", err
				}
				if !decision.Approved {
					reason := decision.Reason
					if reason == "" {
						reason = "denied by approver"
					}
					toolResult = "Tool call denied: " + reason
					toolMsg := model.Message{
						Role:       model.RoleTool,
						Content:    toolResult,
						ToolCallID: tc.ID,
					}
					messages = append(messages, toolMsg)
					if a.cfg.Memory != nil {
						a.cfg.Memory.Add(toolMsg)
					}
					if handler != nil {
						handler("tool", fmt.Sprintf("[%s] %s", tc.Function.Name, toolResult))
					}
					a.emit(ctx, Event{Type: "tool_end", Turn: turn + 1, Tool: tc.Function.Name, Result: toolResult})
					continue
				}
				if decision.Args != "" {
					tc.Function.Arguments = decision.Args
				}
			}
			toolResult = a.callTool(ctx, tc)
			toolMsg := model.Message{
				Role:       model.RoleTool,
				Content:    toolResult,
				ToolCallID: tc.ID,
			}
			messages = append(messages, toolMsg)
			if a.cfg.Memory != nil {
				a.cfg.Memory.Add(toolMsg)
			}
			if handler != nil {
				handler("tool", fmt.Sprintf("[%s] %s", tc.Function.Name, toolResult))
			}
			a.emit(ctx, Event{Type: "tool_end", Turn: turn + 1, Tool: tc.Function.Name, Result: toolResult})
		}

		if a.cfg.Stop != nil && a.cfg.Stop(turn+1, lastContent) {
			a.emit(ctx, Event{Type: "finish", Content: lastContent})
			return lastContent, nil
		}
	}

	if lastContent != "" {
		a.emit(ctx, Event{Type: "finish", Content: lastContent})
		return lastContent, nil
	}
	a.emit(ctx, Event{Type: "finish", Content: "Max turns reached"})
	return "Max turns reached", nil
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	return a.runLoop(ctx, input, nil)
}

func (a *Agent) RunStream(ctx context.Context, input string, handler func(node string, result string) error) error {
	_, err := a.runLoop(ctx, input, handler)
	return err
}

// codingSystemPrompt builds the system prompt for the coding-agent preset.
func codingSystemPrompt(sessionDir string) string {
	workspaceDir := filepath.Join(sessionDir, "workspace")
	return fmt.Sprintf(`You are a coding agent that solves tasks by calling the tools provided to you.

You have tools to read/write/edit/list/delete files, run shell commands, search files (grep), find files (glob), and inspect the workspace. Call a tool when you need to act on the filesystem or run a command; otherwise answer directly.

WORKSPACE: %s

All file paths are relative to the workspace unless absolute. When the task is complete, provide a concise final answer.`, workspaceDir)
}

// ReActCodingAgent is a thin coding-focused preset over the generic Agent.
type ReActCodingAgent struct {
	agent *Agent
}

func NewReActCodingAgent(m model.Model, tools []workflows.Tool, maxIters int, sessionDir string) *ReActCodingAgent {
	return &ReActCodingAgent{
		agent: NewAgent(m, AgentConfig{
			SystemPrompt: codingSystemPrompt(sessionDir),
			Tools:        tools,
			MaxTurns:     maxIters,
		}),
	}
}

func (a *ReActCodingAgent) Run(ctx context.Context, input string) (string, error) {
	return a.agent.Run(ctx, input)
}

func (a *ReActCodingAgent) RunStream(ctx context.Context, input string, handler func(node string, result string) error) error {
	return a.agent.RunStream(ctx, input, handler)
}

// CodingAgent wraps a ReActCodingAgent preset.
type CodingAgent struct {
	react *ReActCodingAgent
}

func (a *CodingAgent) Run(ctx context.Context, input string) (string, error) {
	if a.react != nil {
		return a.react.Run(ctx, input)
	}
	return "", fmt.Errorf("agent not initialized")
}

func (a *CodingAgent) RunStream(ctx context.Context, input string, handler func(node string, result string) error) error {
	if a.react != nil {
		return a.react.RunStream(ctx, input, handler)
	}
	return fmt.Errorf("agent not initialized")
}

func NewCodingAgentWithTools(m model.Model, tools []workflows.Tool, maxIters int, sessionDir string) *CodingAgent {
	return &CodingAgent{
		react: NewReActCodingAgent(m, tools, maxIters, sessionDir),
	}
}
