# Price Tracker Bot - SOLID Principles Refactoring Plan

## Executive Summary

This document outlines a comprehensive refactoring strategy to align the existing Telegram bot with SOLID principles. The current codebase has several violations that impact maintainability, testability, and extensibility.

---

## Current State Analysis & Violations Identified

### **S - Single Responsibility Principle (SRP) Violations**

| File | Issue | Description |
|------|-------|-------------|
| `botfixer/bot_fixer.go` | Too many responsibilities | Handles bot initialization, webhook management, HTTP server setup, and update handling all in one struct |
| `handlers/command_handler.go` | Mixed concerns | Combines command routing, navigation state management, tracker lifecycle, and message formatting |
| `clients/client.go` | Logic leakage | Contains notification criteria processing logic that should be separate from client interface |
| `helpers/messages.go` | Too many functions | 7 different message-related functions with duplicated logic |

### **O - Open/Closed Principle (OCP) Violations**

- Adding new data sources requires modifying existing code in multiple places
- New command types require editing the large switch/if statements in `command_handler.go`
- No abstraction for error handling strategies or retry policies

### **L - Liskov Substitution Principle (LSP) Issues**

- `TrackerBehavior` interface has side effects (sending messages directly) making it hard to substitute implementations
- Mixed return types and error handling patterns across client implementations

### **I - Interface Segregation Principle (ISP) Violations**

- `Client` interface is too broad (`FetchAndExtractData`) with notification logic mixed in
- No separation between data fetching, validation, and notification concerns
- Telegram bot API methods are tightly coupled throughout the codebase

### **D - Dependency Inversion Principle (DIP) Violations**

- High-level modules depend on concrete implementations (e.g., `tgbotapi.BotAPI`)
- Configuration is a global singleton with no abstraction layer
- No dependency injection framework or pattern used

---

## Refactoring Goals by SOLID Principle

### **1. Single Responsibility Principle (SRP)**

**Goal:** Each class should have one reason to change

### **2. Open/Closed Principle (OCP)**

**Goal:** Classes should be open for extension, closed for modification

#### Proposed Changes:

1. **Strategy Pattern Enhancement**
   ```go
   // Current: Direct client instantiation in tracker behavior
   // New: Factory pattern with strategy registry
   
   type DataFetcherFactory interface {
       CreateFetcher(trackerType string) (DataFetcher, error)
   }
   
   type TrackerBehaviorFactory interface {
       CreateBehavior(fetcher DataFetcher, bot *BotAPI) TrackerBehavior
   }
   ```

2. **Command Handler Extension**
   - Introduce `CommandHandlerExtension` interface for plugin-style command additions
   - Use composition over inheritance for new commands

3. **Error Handling Strategy**
   ```go
   type ErrorStrategy interface {
       ShouldRetry(error) bool
       RetryDelay(error) time.Duration
       HandleFatal(error) error
   }
   
   // Default, ExponentialBackoff, CircuitBreaker implementations
   ```

---

### **3. Liskov Substitution Principle (LSP)**

**Goal:** Subtypes must be substitutable for their base types without breaking functionality

#### Proposed Changes:

1. **Pure Interface Design**
   ```go
   // Remove side effects from interface methods
   type DataFetcher interface {
       FetchData(tracker *TrackerConfig) (*PriceData, error)
   }
   
   type NotificationService interface {
       SendNotification(chatID int64, message string) error
       SendInlineKeyboard(chatID int64, keyboard InlineKeyboardMarkup) error
   }

---

### **4. Interface Segregation Principle (ISP)**

**Goal:** Many specific interfaces are better than one general-purpose interface

#### Proposed Changes:

1. **Split Client Interface**
   ```go
   // Current single interface - too broad
   type Client interface {
       FetchAndExtractData(trackerCode string) (*DataResult, error)
   }
   
   // New specialized interfaces
   type PriceExtractor interface {
       ExtractPrice(response []byte) (float64, error)
   }
   
   type CriteriaEvaluator interface {
       Evaluate(value float64, criteria []NotifyCriteria) ([]NotificationTrigger, error)
   }
   
   type MessageBuilder interface {
       BuildNotificationMessage(tracker *TrackerConfig, value float64, triggers []NotificationTrigger) string
   }
   ```

2. **Telegram-Specific Interfaces**
   ```go
   type TelegramSender interface {
       SendMessage(chatID int64, text string, parseMode ParseMode) error
       EditMessage(chatID int64, messageID int, text string) error
       DeleteMessage(chatID int64, messageID int) error
   }
   
   type KeyboardManager interface {
       CreateInlineKeyboard(buttons [][]ButtonData) InlineKeyboardMarkup
       CreateReplyKeyboard(keys [][]KeyboardButton) ReplyKeyboardMarkup
       RemoveKeyboard() RemoveKeyboardMarkup
   }
   ```

---

### **5. Dependency Inversion Principle (DIP)**

**Goal:** High-level modules should not depend on low-level modules; both should depend on abstractions

#### Proposed Changes:

1. **Configuration Abstraction**
   ```go

---

## File Structure After Refactoring

```
price-tracker-bot/
├── main.go                          # Dependency injection setup
├── config/
│   └── config.go                    # Configuration provider implementation
├── infrastructure/                  # Low-level implementations
│   ├── http_client.go               # fasthttp wrapper implementing HTTPClient
│   └── telegram_bot_api.go          # tgbotapi wrapper implementing BotInterface
├── interfaces/                       # Pure interface definitions
│   ├── data_fetcher.go              # DataFetcher, PriceExtractor, CriteriaEvaluator
│   ├── notification.go              # NotificationService, MessageBuilder
│   ├── telegram.go                  # TelegramSender, KeyboardManager
│   └── error_strategy.go            # ErrorStrategy implementations
├── handlers/                        # Business logic layer
│   ├── command_router.go            # Command parsing and routing
│   ├── navigation_manager.go        # State management for user navigation
│   ├── tracker_controller.go        # Tracker lifecycle operations
│   └── message_formatter.go         # Message building utilities
├── services/                        # Service layer (high-level business logic)
│   ├── tracker_service.go           # Tracker orchestration service
│   └── notification_service.go      # Notification orchestration
├── clients/                         # Data fetching implementations
│   ├── api_client.go                # API-based price fetching
│   └── scraper_client.go            # HTML scraping implementation
├── telegram/                        # Telegram-specific functionality
│   ├── sender.go                    # Message sending logic
│   └── keyboard_manager.go          # Keyboard management
├── errors/                          # Error types and handling
│   └── tracker_errors.go            # Custom error types
├── testutils/                       # Testing utilities
│   └── mocks.go                     # Mock implementations for testing
└── refactoring-plans/               # This plan document
    └── solid-refactoring-plan.md
```

---

## Migration Strategy

### **Approach: Incremental Refactoring**

1. **Week 1-2:** Interface redesign and new structure creation (no breaking changes)
2. **Week 3:** Add dependency injection layer alongside existing code
3. **Week 4:** Gradual migration of components to use new interfaces
4. **Testing:** Each phase includes unit tests before removal of old code

### **Risk Mitigation**

- Keep both old and new implementations during transition period
- Use feature flags or environment variables to switch between implementations
- Comprehensive test coverage before removing any production code

---

## Expected Benefits

| Metric | Before | After Refactoring |
|--------|--------|-------------------|
| Cyclomatic Complexity (avg) | High (~15+) | Low (< 8) |
| Lines of Code per File | Large (>400 lines) | Small (<200 lines) |
| Test Coverage | None | >70% |
| New Feature Addition Time | Days | Hours |
| Bug Introduction Risk | High | Low |

---

## Next Steps

1. **Review this plan** - Confirm alignment with project goals
2. **Prioritize phases** - Decide which SOLID principles to address first
3. **Allocate resources** - Estimate time and team needed for each phase
4. **Create detailed task breakdown** - Break down into Jira/GitHub issues

---

## Questions for Stakeholders

1. Is the full 4-week timeline acceptable, or should we prioritize specific SOLID principles?
2. Should we maintain backward compatibility with existing tracker configurations during refactoring?
3. Are there any external dependencies or integrations that must remain unchanged?
4. What is the priority: testability, extensibility, or code organization?

---

**Plan created:** 2026-09-23  
**Estimated effort:** ~160 hours (4 weeks part-time)  
**Risk level:** Medium - requires careful testing and incremental rollout
   type ConfigProvider interface {
       GetTracker(code string) (*TrackerConfig, error)
       GetErrorNotifyLimit() int
       IsEnvironment(env EnvironmentType) bool
   }
   
   // Instead of direct config.GetConfig() usage
   type BotFixer struct {
       botAPIKeyProvider APIKeyProvider
       trackerConfigProvider TrackerConfigProvider
       environmentChecker EnvironmentChecker
   }
   ```

2. **Dependency Injection Container**
   - Introduce a simple DI container for wire dependencies
   - Example: `wire` tool or manual constructor injection

3. **Service Layer Abstraction**
   ```go
   type HTTPClient interface {
       Get(url string) ([]byte, error)
       Post(url string, body []byte) ([]byte, error)
   }
   
   // Instead of direct fasthttp usage
   type TrackerService struct {
       httpClient HTTPClient
       errorStrategy ErrorStrategy
       notificationService NotificationService
   }
   ```

---

## Detailed Refactoring Steps

### **Phase 1: Interface Redesign (Week 1)**

#### Step 1.1: Redefine Core Interfaces
- [ ] Split `Client` into `DataFetcher`, `PriceExtractor`, `CriteriaEvaluator`
- [ ] Create pure `TelegramSender` interface without side effects in method signatures
- [ ] Define `BotInterface` abstraction for Telegram bot API

#### Step 1.2: Service Layer Abstraction
- [ ] Extract `HTTPClient` interface from `services/http_service.go`
- [ ] Create `ErrorStrategy` interface with default implementations
- [ ] Define `NotificationService` interface

**Files to modify:**
- `clients/client.go` → `clients/interfaces.go`, `clients/fetchers.go`
- `helpers/messages.go` → `telegram/telegram_sender.go`, `telegram/keyboard_manager.go`
- `services/http_service.go` → `infrastructure/http_client.go`

---

### **Phase 2: Structural Refactoring (Week 2)**

#### Step 2.1: BotFixer Decomposition
```go
// New structure
type BotManager struct {
    bot *tgbotapi.BotAPI
}

type WebhookServer struct {
    server *http.Server
    handler webhookHandler
}

type UpdateDispatcher struct {
    commandRouter CommandRouter
    navigationManager NavigationManager
}
```

#### Step 2.2: Command Handler Splitting
- Extract `CommandRouter` for command parsing and routing
- Extract `NavigationManager` for state management
- Extract `TrackerController` for tracker lifecycle operations
- Extract `MessageFormatter` for message building

**Files to modify:**
- `handlers/command_handler.go` → 4 new files in `handlers/` package

---

### **Phase 3: Dependency Injection Setup (Week 2)**

#### Step 3.1: Create DI Container
```go
type Container struct {
    botProvider BotProvider
    configProvider ConfigProvider
    httpClientProvider HTTPClientProvider
}

func NewContainer() *Container { ... }
```

#### Step 3.2: Wire Dependencies
- Update all structs to use interfaces instead of concrete types
- Implement constructor functions that accept dependencies

**Files to modify:**
- All struct definitions across the codebase
- `main.go` → dependency injection setup

---

### **Phase 4: Error Handling & Logging (Week 3)**

#### Step 4.1: Unified Error Types
```go
type TrackerError struct {
    Code string
    Message string
    Cause error
}

type BotError struct {
    ChatID int64
    Message string
}
```

#### Step 4.2: Logger Abstraction
- Introduce `Logger` interface (or use standard log with abstraction)
- Create structured logging utilities

**Files to modify:**
- Add `errors/` package for error types
- Update all error handling throughout codebase

---

### **Phase 5: Testing Infrastructure (Week 3)**

#### Step 5.1: Mock Interfaces
```go
// For testing
type MockTelegramSender struct{}
func (m *MockTelegramSender) SendMessage(...) error { ... }
```

#### Step 5.2: Test Utilities
- Create `testutils/` package with test helpers
- Add table-driven tests for all interfaces

---

### **Phase 6: Cleanup & Migration (Week 4)**

#### Step 6.1: Remove Deprecated Code
- Eliminate old helper functions no longer needed
- Clean up unused imports and variables

#### Step 6.2: Documentation Update
- Add godoc comments to all new interfaces
- Create migration guide for existing code

   ```

2. **Dependency Injection for Bot API**
   - Inject `BotAPI` as a dependency rather than storing it in every struct
   - Use interface-based bot abstraction:
     ```go
     type BotInterface interface {
         Send(message Message) (Message, error)
         EditText(chatID int64, messageID int, text string) (Message, error)
         DeleteMessage(chatID int64, messageID int) error
     }
     ```


#### Proposed Changes:

| Current Structure | New Structure | Rationale |
|------------------|---------------|-----------|
| `BotFixer` struct | Split into: `BotManager`, `WebhookServer`, `UpdateDispatcher` | Separate concerns of bot lifecycle, HTTP server, and update routing |
| `CommandHandler` | Split into: `CommandRouter`, `NavigationManager`, `TrackerController`, `MessageFormatter` | Each handles one aspect of command processing |
| `Client` interface + notification logic | Separate interfaces: `DataFetcher`, `NotificationService` | Data fetching should not include business logic about notifications |
| `helpers/messages.go` | Split into: `TelegramSender`, `KeyboardManager`, `MessageBuilder` | Group related functionality |
