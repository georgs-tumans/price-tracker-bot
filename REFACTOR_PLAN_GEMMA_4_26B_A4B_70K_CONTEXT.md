# Refactoring Plan for Price Tracker Bot

**Model ID:** `GEMMA_4_26B_A4B_70K_CONTEXT`

#### 1. Overview
The current codebase is a functional Go-based Telegram bot, but it exhibits several architectural patterns that violate SOLID principles, particularly regarding Single Responsibility and Open/Closed principles. This plan outlines the steps to refactor the core components to improve maintainability, testability, and extensibility.

#### 2. Identified Violations & Proposed Solutions

| Principle | Violation Found | Proposed Refactoring |
| :--- | :--- | :--- |
| **Single Responsibility (SRP)** | `CommandHandler` is responsible for command routing, navigation state management, AND managing the lifecycle of running trackers (`runningTrackers`, `AddRunningTracker`, etc.). | Extract tracker lifecycle management into a new `TrackerManager` service. `CommandHandler` will delegate all tracker-related operations to this manager. |
| **Open/Closed (OCP)** | `CreateTracker` in `handlers/tracker.go` uses a hardcoded `switch` statement to instantiate different `TrackerBehavior` types based on configuration. Adding new tracker types requires modifying this function. | Implement a **Tracker Factory** pattern. Register tracker type creators at runtime or via a registry, allowing new tracker types (e.g., Webhook-based) to be added without modifying the core creation logic. |
| **Liskov Substitution (LSP)** | While `TrackerBehavior` is an interface, we must ensure that all implementations can be used interchangeably within the `Tracker` struct without side effects. | Conduct a review of all `TrackerBehavior` implementations to ensure they adhere strictly to the contract defined by the interface. |
| **Interface Segregation (ISP)** | The `Messenger` interface is currently quite comprehensive, but we must ensure that components only depend on the specific methods they need. | Review usage patterns in `CommandHandler` and `Tracker`. If certain components only need to *send* messages while others only need to *edit* them, consider splitting the interface. |
| **Dependency Inversion (DIP)** | The initialization of the bot in `BotFixer` is somewhat tightly coupled with concrete implementations during construction. | Enhance dependency injection by passing pre-constructed services (like `TrackerManager`, `Messenger`, and `Config`) into constructors rather than having them instantiate their own dependencies deep within the hierarchy. |

#### 3. Implementation Roadmap

**Phase 1: Core Infrastructure**
*   [ ] Create `pkg/tracker/manager.go` to house the `TrackerManager`.
*   [ ] Define a `TrackerFactory` interface and implement a registry-based factory.
*   [ ] Refactor `handlers/tracker.go` to use the new factory for behavior instantiation.

**Phase 4: Command Handler Decoupling**
*   [ ] Update `CommandHandler` struct to replace `runningTrackers []*Tracker` with a reference to `TrackerManager`.
*   [ ] Refactor all command handlers (run, stop, status, etc.) to call methods on `TrackerManager` instead of managing the slice directly.

**Phase 3: Dependency Injection Cleanup**
*   [ ] Refactor `BotFixer` and `main.go` to follow a "top-down" dependency injection pattern where all services are initialized in `main` and passed down.

#### 4. Expected Outcomes
*   **Improved Testability:** By isolating tracker management and using interfaces for messaging, we can easily mock these components in unit tests.
*   **Easier Extensibility:** Adding a new type of data source (e.g., a database-driven scraper) will only require adding a new `TrackerBehavior` implementation and registering it with the factory.
*   **Reduced Complexity:** The `CommandHandler` will become significantly smaller and focused solely on command parsing and routing.
