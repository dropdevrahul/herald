package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

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
	// ToolTimeout is the per-call deadline applied to each tool execution.
	// A value of 0 means no timeout.
	ToolTimeout time.Duration
	// Checkpointer persists thread state for the durable RunThread/Resume flow.
	// It is required when InterruptBefore is used.
	Checkpointer workflows.Checkpointer
	// InterruptBefore, when non-nil and returning true for a tool call, pauses
	// the run durably before that call: thread state is saved via Checkpointer
	// and RunThread returns an AgentResult with a non-nil Interrupt. The caller
	// resumes with Resume once a human decision is available.
	InterruptBefore func(call model.ToolCall) bool
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

// invokeTool runs tool.Call inside a goroutine, recovering from panics and
// honouring ctx cancellation. The result or error is returned as a string.
func (a *Agent) invokeTool(ctx context.Context, tool workflows.Tool, args string) string {
	type result struct {
		out string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		// ponytail: the timed-out tool goroutine leaks until it returns; bound the work inside the tool if that matters
		defer func() {
			if r := recover(); r != nil {
				ch <- result{err: fmt.Errorf("tool panicked: %v", r)}
			}
		}()
		out, err := tool.Call(ctx, args)
		ch <- result{out: out, err: err}
	}()
	select {
	case <-ctx.Done():
		return "Error: " + ctx.Err().Error()
	case res := <-ch:
		if res.err != nil {
			return "Error: " + res.err.Error()
		}
		if res.out == "" {
			return "Tool executed"
		}
		return res.out
	}
}

func (a *Agent) callTool(ctx context.Context, tc model.ToolCall) string {
	for _, tool := range a.cfg.Tools {
		if tc.Function.Name == tool.Name() {
			args := tc.Function.Arguments
			if args == "" {
				args = "{}"
			}
			callCtx := ctx
			if a.cfg.ToolTimeout > 0 {
				var cancel context.CancelFunc
				callCtx, cancel = context.WithTimeout(ctx, a.cfg.ToolTimeout)
				defer cancel()
			}
			return a.invokeTool(callCtx, tool, args)
		}
	}
	return "Tool not found: " + tc.Function.Name
}

// AgentResult is returned by RunResult, carrying the final content together
// with aggregated token usage across all turns and the number of turns executed.
type AgentResult struct {
	Content string
	Usage   model.Usage
	Turns   int
	// Interrupt is non-nil when a durable RunThread/Resume run paused awaiting a
	// human decision on Interrupt.ToolCall. Resume with the same threadID to continue.
	Interrupt *Interrupt
}

// Interrupt describes a durable pause: the run persisted its thread state and is
// awaiting a human decision on ToolCall before it can continue.
type Interrupt struct {
	ThreadID string
	ToolCall model.ToolCall
}

// runLoop is the shared multi-turn engine. handler may be nil; when set it is
// invoked with ("content", delta) for streamed text and ("tool", "[name] result")
// after each tool call.
func (a *Agent) runLoop(ctx context.Context, input string, handler func(node string, result string) error) (AgentResult, error) {
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

	var agg model.Usage
	lastContent := ""
	for turn := 0; turn < maxTurns; turn++ {
		a.emit(ctx, Event{Type: "turn_start", Turn: turn + 1})
		resultChan := a.model.Stream(ctx, messages, opts)

		var content string
		var toolCalls []model.ToolCall

		for result := range resultChan {
			if result.Err != nil {
				return AgentResult{}, result.Err
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
			if result.Usage.TotalTokens > 0 {
				agg.PromptTokens += result.Usage.PromptTokens
				agg.CompletionTokens += result.Usage.CompletionTokens
				agg.TotalTokens += result.Usage.TotalTokens
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
			return AgentResult{Content: content, Usage: agg, Turns: turn + 1}, nil
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
					return AgentResult{}, err
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
			return AgentResult{Content: lastContent, Usage: agg, Turns: turn + 1}, nil
		}
	}

	if lastContent != "" {
		a.emit(ctx, Event{Type: "finish", Content: lastContent})
		return AgentResult{Content: lastContent, Usage: agg, Turns: maxTurns}, nil
	}
	a.emit(ctx, Event{Type: "finish", Content: "Max turns reached"})
	return AgentResult{Content: "Max turns reached", Usage: agg, Turns: maxTurns}, nil
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	r, err := a.runLoop(ctx, input, nil)
	return r.Content, err
}

func (a *Agent) RunStream(ctx context.Context, input string, handler func(node string, result string) error) error {
	_, err := a.runLoop(ctx, input, handler)
	return err
}

// RunResult runs the agent and returns an AgentResult that includes the final
// content, aggregated token usage across all turns, and the number of turns
// executed.
func (a *Agent) RunResult(ctx context.Context, input string) (AgentResult, error) {
	return a.runLoop(ctx, input, nil)
}

// threadState is the persisted shape of a durable run. Pending holds the tool
// calls of the current turn that have not yet executed; Pending[0] is the one a
// pending Interrupt is awaiting a decision on.
type threadState struct {
	Messages []model.Message  `json:"messages"`
	Turn     int              `json:"turn"`
	Pending  []model.ToolCall `json:"pending"`
}

// streamTurn collects a single model stream into its content, tool calls, and
// token usage.
func (a *Agent) streamTurn(ctx context.Context, messages []model.Message, opts *model.ModelOptions) (string, []model.ToolCall, model.Usage, error) {
	var content string
	var toolCalls []model.ToolCall
	var usage model.Usage
	for result := range a.model.Stream(ctx, messages, opts) {
		if result.Err != nil {
			return "", nil, model.Usage{}, result.Err
		}
		if result.Delta != "" {
			content += result.Delta
		}
		if result.Content != "" && result.Delta == "" {
			content += result.Content
		}
		if len(result.ToolCalls) > 0 {
			toolCalls = result.ToolCalls
		}
		if result.Usage.TotalTokens > 0 {
			usage.PromptTokens += result.Usage.PromptTokens
			usage.CompletionTokens += result.Usage.CompletionTokens
			usage.TotalTokens += result.Usage.TotalTokens
		}
	}
	return content, toolCalls, usage, nil
}

// execToolCall runs the Approver gate (if any) then the tool, returning the
// RoleTool message to append (a denial message or the tool result).
func (a *Agent) execToolCall(ctx context.Context, turn int, tc model.ToolCall) (model.Message, error) {
	a.emit(ctx, Event{Type: "tool_start", Turn: turn, Tool: tc.Function.Name, Args: tc.Function.Arguments})
	if a.cfg.Approver != nil {
		decision, err := a.cfg.Approver(ctx, tc)
		if err != nil {
			return model.Message{}, err
		}
		if !decision.Approved {
			reason := decision.Reason
			if reason == "" {
				reason = "denied by approver"
			}
			result := "Tool call denied: " + reason
			a.emit(ctx, Event{Type: "tool_end", Turn: turn, Tool: tc.Function.Name, Result: result})
			return model.Message{Role: model.RoleTool, Content: result, ToolCallID: tc.ID}, nil
		}
		if decision.Args != "" {
			tc.Function.Arguments = decision.Args
		}
	}
	result := a.callTool(ctx, tc)
	a.emit(ctx, Event{Type: "tool_end", Turn: turn, Tool: tc.Function.Name, Result: result})
	return model.Message{Role: model.RoleTool, Content: result, ToolCallID: tc.ID}, nil
}

// saveThread persists thread state under threadID via the configured Checkpointer.
func (a *Agent) saveThread(ctx context.Context, threadID string, st threadState) error {
	if a.cfg.Checkpointer == nil {
		return errors.New("interrupt requested but no Checkpointer configured")
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return a.cfg.Checkpointer.Save(ctx, workflows.Checkpoint{
		ThreadID:  threadID,
		Node:      "agent",
		Iteration: st.Turn,
		State:     raw,
	})
}

// drive is the durable multi-turn engine shared by RunThread and Resume. It
// drains any pending tool calls (pausing on InterruptBefore), then streams the
// next model turn, repeating until the model stops requesting tools, a stop
// condition fires, the turn budget is exhausted, or an interrupt is raised.
func (a *Agent) drive(ctx context.Context, threadID string, st threadState) (AgentResult, error) {
	opts := a.modelOptions()
	maxTurns := a.maxTurns()

	messages := st.Messages
	turn := st.Turn
	pending := st.Pending
	var agg model.Usage
	lastContent := ""

	for turn < maxTurns {
		// Drain pending tool calls from the current turn, pausing if requested.
		for len(pending) > 0 {
			tc := pending[0]
			if a.cfg.InterruptBefore != nil && a.cfg.InterruptBefore(tc) {
				if err := a.saveThread(ctx, threadID, threadState{Messages: messages, Turn: turn, Pending: pending}); err != nil {
					return AgentResult{}, err
				}
				return AgentResult{
					Content:   lastContent,
					Usage:     agg,
					Turns:     turn,
					Interrupt: &Interrupt{ThreadID: threadID, ToolCall: tc},
				}, nil
			}
			toolMsg, err := a.execToolCall(ctx, turn, tc)
			if err != nil {
				return AgentResult{}, err
			}
			messages = append(messages, toolMsg)
			if a.cfg.Memory != nil {
				a.cfg.Memory.Add(toolMsg)
			}
			pending = pending[1:]
		}

		a.emit(ctx, Event{Type: "turn_start", Turn: turn + 1})
		content, toolCalls, usage, err := a.streamTurn(ctx, messages, opts)
		if err != nil {
			return AgentResult{}, err
		}
		agg.PromptTokens += usage.PromptTokens
		agg.CompletionTokens += usage.CompletionTokens
		agg.TotalTokens += usage.TotalTokens
		turn++
		lastContent = content
		a.emit(ctx, Event{Type: "model_response", Turn: turn, Content: content})

		if len(toolCalls) == 0 {
			if a.cfg.Memory != nil {
				a.cfg.Memory.Add(model.Message{Role: model.RoleAssistant, Content: content})
			}
			a.emit(ctx, Event{Type: "finish", Content: content})
			return AgentResult{Content: content, Usage: agg, Turns: turn}, nil
		}

		assistantMsg := model.Message{Role: model.RoleAssistant, Content: content, ToolCalls: toolCalls}
		messages = append(messages, assistantMsg)
		if a.cfg.Memory != nil {
			a.cfg.Memory.Add(assistantMsg)
		}
		pending = toolCalls

		if a.cfg.Stop != nil && a.cfg.Stop(turn, lastContent) {
			a.emit(ctx, Event{Type: "finish", Content: lastContent})
			return AgentResult{Content: lastContent, Usage: agg, Turns: turn}, nil
		}
	}

	a.emit(ctx, Event{Type: "finish", Content: lastContent})
	return AgentResult{Content: lastContent, Usage: agg, Turns: turn}, nil
}

// RunThread starts a durable run identified by threadID. When InterruptBefore
// pauses the run, the returned AgentResult has a non-nil Interrupt and the
// thread state is persisted via the configured Checkpointer; call Resume with
// the same threadID to continue.
func (a *Agent) RunThread(ctx context.Context, threadID string, input string) (AgentResult, error) {
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
	return a.drive(ctx, threadID, threadState{Messages: messages})
}

// Resume continues a paused durable run. It loads the persisted thread state for
// threadID, applies the human decision to the pending tool call (executing it,
// rewriting its arguments, or denying it), and drives the run forward — possibly
// in a different process from the one that paused it.
func (a *Agent) Resume(ctx context.Context, threadID string, decision ApprovalDecision) (AgentResult, error) {
	if a.cfg.Checkpointer == nil {
		return AgentResult{}, errors.New("Resume: no Checkpointer configured")
	}
	cp, ok, err := a.cfg.Checkpointer.Load(ctx, threadID)
	if err != nil {
		return AgentResult{}, err
	}
	if !ok {
		return AgentResult{}, fmt.Errorf("Resume: no checkpoint for thread %q", threadID)
	}
	var st threadState
	if err := json.Unmarshal(cp.State, &st); err != nil {
		return AgentResult{}, err
	}
	if len(st.Pending) == 0 {
		return AgentResult{}, fmt.Errorf("Resume: thread %q has no pending tool call", threadID)
	}

	tc := st.Pending[0]
	var toolMsg model.Message
	if !decision.Approved {
		reason := decision.Reason
		if reason == "" {
			reason = "denied by human"
		}
		toolMsg = model.Message{Role: model.RoleTool, Content: "Tool call denied: " + reason, ToolCallID: tc.ID}
		a.emit(ctx, Event{Type: "tool_end", Turn: st.Turn, Tool: tc.Function.Name, Result: toolMsg.Content})
	} else {
		if decision.Args != "" {
			tc.Function.Arguments = decision.Args
		}
		a.emit(ctx, Event{Type: "tool_start", Turn: st.Turn, Tool: tc.Function.Name, Args: tc.Function.Arguments})
		result := a.callTool(ctx, tc)
		toolMsg = model.Message{Role: model.RoleTool, Content: result, ToolCallID: tc.ID}
		a.emit(ctx, Event{Type: "tool_end", Turn: st.Turn, Tool: tc.Function.Name, Result: result})
	}
	st.Messages = append(st.Messages, toolMsg)
	if a.cfg.Memory != nil {
		a.cfg.Memory.Add(toolMsg)
	}
	st.Pending = st.Pending[1:]

	return a.drive(ctx, threadID, st)
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
