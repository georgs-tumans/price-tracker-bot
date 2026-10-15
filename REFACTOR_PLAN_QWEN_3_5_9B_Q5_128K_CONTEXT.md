
## Executive Summary

This refactoring plan addresses violations of SOLID principles in the price-tracker-bot codebase.

### Current Issues Identified:
- **SRP**: CommandHandler does too many things (command parsing, navigation, tracker lifecycle, state persistence)
- **OCP**: Adding new client types requires modifying switch statements
- **DIP**: HTTP client used directly instead of through interface
- **ISP**: Messenger interface has methods not all components need
- **LSP**: Tracker mixes config data with runtime state confusingly

---

## Detailed Analysis by SOLID Principle

### 1. Single Responsibility Principle (SRP)

#### 1.1 CommandHandler
- Responsibilities: command parsing, navigation state, tracker lifecycle, state persistence, messaging
- Problem: Too many unrelated concerns in one class
- Solution: Split into CommandRegistry, CommandParser, NavigationManager, TrackerRegistry, StatePersistence, CommandExecutor

#### 1.2 Tracker
- Responsibilities: coordination, state management, error handling, backoff logic, messaging
- Problem: Mixes coordination with stateful behavior
- Solution: Split into Coordinator, StateManager, BackoffPolicy interface, ErrorNotifier interface

#### 1.3 Configuration
- Responsibilities: env var loading, file I/O, validation, lookups, permissions
- Problem: Mixes file I/O with data access and validation

---

### 2. Open/Closed Principle (OCP)

#### 2.1 Client Abstraction
- Problem: Adding new client types requires modifying switch statements in tracker.go
- Solution: Factory pattern with registration-based approach
  - Define DataFetcher interface for all clients
  - Create ClientFactory that registers factories by name

#### 2.2 Tracker Behavior Extensibility
- Problem: Concrete behaviors tightly coupled to config types
- Solution: Abstract data access from behavior logic with DataAccessor interface and BehaviorFactory

---

### 3. Dependency Inversion Principle (DIP)

#### 3.1 HTTP Client Abstraction
- Problem: Concrete httpClient used directly, preventing easy mocking in tests
- Solution: Define HTTPClient interface at service layer with Get() method
  - Inject into client constructors instead of creating internally

#### 3.2 Messenger Abstraction
- Already implemented correctly with interface and dependency injection
- Continue using this pattern consistently for all dependencies

---

### 4. Interface Segregation Principle (ISP)

#### 4.1 Messenger Interface Refinement
- Problem: Single interface has methods not all components need
- Solution: Split into MessageSender, MessageEditor, KeyboardManager interfaces
  - Each component depends only on what it actually uses

#### 4.2 TrackerBehavior Interface
- Problem: Execute method returns too much data (value, notification, error) at once
- Solution: Split into ValueExtractor and NotificationBuilder interfaces

---

### 5. Liskov Substitution Principle (LSP)

#### 5.1 Type Safety Issues
- Problem: Tracker mixes config.Tracker directly with runtime state confusingly
- Solution: Use ConfiguredTracker interface instead of concrete type
  - Separate immutable config from mutable runtime state

---

## Implementation Priority (12-week plan)

### Phase 1: Foundation (Weeks 1-2)
- Create HTTPClient and Messenger interfaces
- Implement dependency injection framework
- Write integration tests to verify architecture

### Phase 2: Command Handler Refactoring (Weeks 3-4)
- Split into focused components
- Implement NavigationManager separately

### Phase 3: Tracker Refactoring (Weeks 5-6)
- Separate Coordinator, StateManager, ErrorNotifier
- Implement BackoffPolicy interface with strategies

### Phase 4: Client Extensions (Weeks 7-8)
- Factory pattern for clients
- Add example new client type (e.g., GraphQL)

### Phase 5: Configuration Refactoring (Weeks 9-10)
- Split into focused components

### Phase 6: Cleanup and Polish (Weeks 11-12)
- Remove deprecated code
- Update all tests to new architecture

- Documentation updates

---

## Testing Strategy

- Unit tests with mocks for each refactored component
- Table-driven tests for behavior variations
- Integration tests with mock HTTP server

---

## Migration Checklist

- [ ] All existing functionality preserved
- [ ] All unit tests passing (including race detection)
- [ ] New client types can be added without modifying core logic
- [ ] State persistence works after restart

---

## Risk Assessment

**Low Risk**:
- Interface additions (DIP, ISP) - backward compatible
**Medium Risk**:
- Tracker lifecycle refactoring - requires careful testing of goroutines

---

## Success Metrics

1. **Maintainability**: Adding new client types requires < 50 lines of code and no switch statements
2. **Test Coverage**: All refactored components have > 80% test coverage with race-free tests
3. **Build Time**: No significant performance degradation (< 5% increase in cold start time)
