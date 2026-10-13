# Refactoring Plan: Improving SOLID Compliance in Price Tracker Bot

## 1. Introduction & Goals
The goal of this refactoring is to transition the `price-tracker-bot` from a monolithic, tightly coupled architecture to a modular, testable, and extensible one by adhering to **SOLID** principles.

**Primary Objectives:**
*   **Improve Testability**: Enable easy mocking of clients, messengers, and managers.
*   **Enhance Extensibility**: Allow adding new tracker types (e.g., specialized scrapers) without modifying core logic.
*   **Reduce Complexity**: Break down "God Objects" like `CommandHandler` into smaller, focused components.

---

## 2. Detailed Analysis of Current Violations

### **S - Single Responsibility Principle (SRP)**
*   **`handlers/command_handler.go`**: Currently acts as a router, a tracker manager, and a navigation state keeper. It has too many reasons to change.
    *   *Risk:* As more commands or complex navigation features are added, this file will become a "God Object," making it harder to maintain and test.
*   **`handlers/tracker.go`**: The `Tracker` struct manages its own goroutine (`run`), handles backoff logic, maintains status, and orchestrates the execution via `TrackerBehavior`.
    *   *Risk:* It mixes state management (status/backoff) with orchestration (timing/execution).

### **O - Open/Closed Principle (OCP)**
*   **`handlers/tracker.go:CreateTracker`**: Uses a hardcoded `switch` statement on `trackerType` to decide which behavior to instantiate (`NewAPITrackerBehavior` or `NewScraperTrackerBehavior`).
    *   *Risk:* Every time you add a new type of client (e.g., a specialized web scraper), you must modify this existing function, increasing the risk of breaking existing logic.

### **L - Liskov Substitution Principle (LSP)**
*   While interfaces like `Client` exist, we must ensure that all implementations (`PublicAPIClient`, `ScraperClient`) behave identically regarding error handling and return values so they can be swapped seamlessly within a `Tracker`.

### **I - Interface Segregation Principle (ISP)**
*   Current components often depend on large structs. For example, `CommandHandler` depends on the full `config.Configuration`. We should move towards smaller interfaces for specific needs (e.g., an `IntervalProvider` interface).

### **D - Dependency Inversion Principle (DIP)**
*   **Hardcoded Dependencies**: Many components instantiate their own dependencies (e.g., `NewAPITrackerBehavior` creates its own client). This prevents dependency injection and makes unit testing difficult without real network calls.
*   **Concrete vs Abstract**: High-level modules like `CommandHandler` depend on concrete types like `Tracker` instead of an abstraction/interface.

---

## 3. Proposed Architecture Changes

### **A. Component Decomposition**
We will split the current large structs into specialized managers:

1.  **`CommandRouter`**: Responsible *only* for parsing incoming Telegram messages and dispatching them to the correct handler function.
2.  **`TrackerManager`**: Manages the lifecycle of trackers (Start, Stop, List, Get). It will hold the `runningTrackers` map/slice.
3.  **`NavigationManager`**: Encapsulates the logic for per-chat menu history and "Return" button states.

### **B. Pattern Implementation**
1.  **Behavior Factory**: Implement a registry-based factory where different `TrackerBehavior` implementations can be registered at startup. This allows adding new types without touching the factory core.
2.  **Dependency Injection (DI)**: All components will receive their dependencies via constructors (`New...` functions).

### **C. New Interface Definitions**
*   `type TrackerManager interface { Start(code string) error; Stop(code string) error; ... }`
*   `type NavigationProvider interface { GetState(chatID int64) *NavigationState; ... }`
*   `type BehaviorFactory interface { Create(trackerType string) (TrackerBehavior, error) }`

---

## 4. Implementation Roadmap

### **Phase 1: Decoupling & Responsibility Splitting**
*   [ ] Extract `TrackerManager` from `CommandHandler`.
*   [ ] Extract `NavigationManager` from `CommandHandler`.
*   [ ] Refactor `CommandHandler` to use these new managers via interfaces.

### **Phase 2: Dependency Injection & Abstraction**
*   [ ] Update all constructors to accept interfaces instead of concrete types.
*   [ ] Introduce mocks for `Messenger`, `Client`, and `TrackerManager` to enable robust unit testing.

### **Phase 3: Open/Closed Refactoring (Factory Pattern)**
*   [ ] Implement a registry-based `BehaviorFactory`.
*   [ ] Remove the `switch` statement from `CreateTracker`.

### **Phase 4: Final Cleanup & Validation**
*   [ ] Run all existing tests.
*   [ ] Add new unit tests for the newly extracted managers.
*   [ ] Perform a final linting check with `golangci-lint`.

---

## 5. Testing Strategy
*   **Unit Tests**: Focus on testing individual logic in `TrackerManager` and `NavigationManager` using mocks.
*   **Integration Tests**: Test the interaction between `CommandHandler` and the new managers.
*   **Race Detection**: Always run tests with `-race` to ensure concurrency remains safe after splitting components.
