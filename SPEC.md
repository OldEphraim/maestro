# Maestro — Project Specification

## Problem Statement

Build a platform where users can create AI agents, configure how they behave and operate (personality, tools, schedules, memory, limits), and connect them into collaborative workflows. Agents must run on a real runtime, execute real tools, and communicate with each other to complete tasks autonomously. At least one agent must be reachable through an external messaging channel (WhatsApp, Telegram, or Slack) so a human can interact with it conversationally. The platform must include a web UI for managing everything visually.

## Evaluation Weights

| Criteria | Weight |
|---|---|
| Working end-to-end demo | 40% |
| Architecture and code quality | 30% |
| UI/UX and configurability | 20% |
| Documentation | 10% |

## Key Impact Metrics

- Number of configurable dimensions per agent
- Time from zero to a working multi-agent workflow
- End-to-end task completion rate
- Agent-to-agent message reliability

## Technical Requirements

**Programming Languages:** Candidate's choice — must justify in README.

**AI/ML Frameworks — must integrate one:**
- OpenClaw (https://openclaw.ai) — Always-on personal agent framework
- OpenCode (https://opencode.ai) — Terminal-native coding agent
- Goose (https://block.github.io/goose/) — Open-source AI agent by Block

Candidate must choose one and explain the tradeoff in the README.

**Development Tools — must include:**
- A web-based UI
- A persistence layer (agent configs, memory, workflow state, execution history)
- Real-time communication between frontend and backend (WebSocket or SSE)

**Cloud Platforms:** Optional — the project must run fully local with a single setup command.

## Agent Communication Requirements

1. When Agent A finishes work, it sends a message to Agent B with the output
2. Agent B picks it up, processes it, and either passes it forward or sends it back with feedback
3. The platform must persist the message history so the UI can show the full conversation trail between agents

At least one agent must be connected to an external messaging channel (WhatsApp, Telegram, or Slack). The chosen runtime must actually execute the agent logic — **this is not a UI mockup exercise.**

## Functional Requirements

**Agent CRUD** — create, edit, delete agents from the UI. Each agent has:
- Name, Role, System prompt, Model, Tool access, Communication channels

**Agent Configuration** — for each agent, configure:
- Schedules — cron jobs or intervals that wake the agent
- Memory — persistent facts and preferences retained across sessions
- Skills — reusable step-by-step procedures combining multiple tools
- Interaction rules — what the agent can do autonomously vs. what requires approval
- Guardrails — cost limits, rate limits, blocked actions

**Workflow Builder** — connect agents into workflows visually. Define execution order, conditions, and feedback loops. Example — "Dev Environment" workflow:
1. A Coder agent writes code
2. A Reviewer agent reviews it
3. If rejected → sends feedback back to Coder
4. If approved → a Deployer agent deploys it

This loop must be configurable, not hardcoded.

**Workflow Templates** — at least 2 pre-built templates that users can load and modify.

**External Channel Integration** — at least one agent accessible through WhatsApp, Telegram, or Slack:
- The user must be able to chat with the agent from the messaging app
- The agent must respond using its configured personality, tools, and memory
- The UI should show which channel each agent is connected to

**Live Monitoring** — real-time view of:
- Agent status, logs, inter-agent messages (including from external channels)
- Task progress, basic token/cost tracking

## Working End-to-End Demo

At least one workflow with 2+ agents must execute a real task:
- Agents must actually call tools, produce output, exchange messages, and reach a conclusion
- Additionally, demo a human chatting with one of the agents through the connected messaging channel

## Code Quality Expectations

- Clear separation between UI layer, agent runtime integration, and data/persistence layer
- Tests for critical paths (agent creation, workflow execution, message delivery)
- README with: architecture diagram, setup instructions, runtime choice justification, instructions for adding a new workflow template or a new messaging channel

## Performance Benchmarks

N/A — focus is on functionality and architecture, not specific numbers. It should feel responsive and work smoothly.