The Problem: Reliable State Persistence and Checkpointing for Agentic AI

Developers building agentic AI systems frequently encounter issues with reliably persisting and checkpointing agent state. This is critical for long-running agents, agents performing complex multi-step operations, or when an agent needs to recover from failures (e.g., a connection drop, application crash, or user cancellation). Current solutions often lack robustness, leading to lost state, inconsistent memory, and difficulties in resuming operations. This is particularly challenging when agents interact with external databases or stream data, where partial updates or uncommitted transactions can leave the system in an unrecoverable state.

Why this project shape/stack?

This prototype is a Go service designed to act as a dedicated "Agent State Store." Go was chosen for its excellent concurrency primitives, strong typing, and performance characteristics, making it suitable for a reliable backend service. A service-based approach decouples state management from the agent's core logic, allowing agents to be stateless or minimal-state, relying on this external service for durable persistence. This also enables centralized state management, versioning, and potential integration with various agent frameworks without tight coupling.

The service provides a simple HTTP API for `PUT` (save), `GET` (retrieve), and `DELETE` (clear) agent states, along with a `CHECKPOINT` operation that ensures an atomic update of an agent's state. It uses a file-based storage mechanism by default for simplicity and zero-dependency setup, demonstrating the core concept without requiring an external database. This makes it immediately usable with zero API keys or external service configurations.

Setup and Usage Instructions (Zero API keys required):

1.  **Clone the repository:**
    `git clone <this-repo-url>`
    `cd <this-repo-directory>`

2.  **Run the service:**
    `go run .`
    The service will start on `http://localhost:8080`.

3.  **Interact with the service (using `curl`):**

    *   **Save Agent State:**
        `curl -X PUT -H "Content-Type: application/json" -d '{"agent_id": "agent-123", "version": 1, "state": {"step": 1, "task": "research", "data": {"query": "AI agents"}}}' http://localhost:8080/state`

    *   **Get Agent State:**
        `curl http://localhost:8080/state/agent-123`

    *   **Checkpoint Agent State (Atomic Update):**
        This simulates updating an agent's state, ensuring the old state is only replaced if the provided `version` matches the current stored version. If the `version` in the payload is `0`, it's treated as an initial save or an unconditional update.

        First, save an initial state (or use the one from above):
        `curl -X PUT -H "Content-Type: application/json" -d '{"agent_id": "agent-456", "version": 0, "state": {"step": 0, "status": "initialized"}}' http://localhost:8080/state`

        Now, checkpoint with an updated state and the correct version:
        `curl -X POST -H "Content-Type: application/json" -d '{"agent_id": "agent-456", "version": 0, "newState": {"step": 1, "status": "processing", "progress": 0.5}}' http://localhost:8080/checkpoint`
        *(Note: The 'version' in the payload for checkpoint is the *expected current version* to match against before updating. `0` here means "if no state exists or if state is version 0, update it")*

        To demonstrate optimistic locking, try to checkpoint `agent-456` with an incorrect version:
        `curl -X POST -H "Content-Type: application/json" -d '{"agent_id": "agent-456", "version": 999, "newState": {"step": 2, "status": "completed"}}' http://localhost:8080/checkpoint`
        This will return a `409 Conflict` because the expected version `999` does not match the current version.

    *   **Delete Agent State:**
        `curl -X DELETE http://localhost:8080/state/agent-123`

Optional Real-LLM Adapter:

This project is purely about state persistence and has no direct LLM involvement. Therefore, no LLM adapter is provided or needed. The `state` field in the JSON payload is a generic `map[string]interface{}` (or `any` in JSON terms), capable of storing any data an LLM-driven agent might generate or use.
