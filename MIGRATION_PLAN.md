# Dependency Migration Analysis Plan

**Date**: September 23, 2026  
**Project**: Price Tracker Bot (pricetrackerbot)

---

## Executive Summary

This document analyzes all dependencies currently used in your project, evaluates available alternatives, assesses migration effort, and provides recommendations for modernizing the codebase.

---

## Current Dependencies Overview

### 1. Go Version
| Aspect | Current Status |
|--------|---------------|
| **Version** | `go 1.27` (updated from 1.23.2) ✅ |
| **System Installed** | `go1.27.1 linux/amd64` ✅ |
| **Status** | Already updated, no action needed |

---

### 2. Telegram Bot API (`github.com/go-telegram-bot-api/telegram-bot-api/v5`)  
**Current Version**: `v5.5.1` (released Dec 2024)

#### Usage in Codebase:
```go
// helpers/ui_helper.go (~15 lines of usage)
tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
func GetReturnButtonMenu(existingMenu *tgbotapi.InlineKeyboardMarkup) ...

// botfixer/bot_fixer.go (~40 lines of usage)
type BotFixer struct {
    Bot *tgbotapi.BotAPI
}
func NewBotFixer() *BotFixer {
    b.Bot, err = tgbotapi.NewBotAPI(botFixer.Config.BotAPIKey)
}
```

#### Analysis:
| Aspect | Details |
|--------|----------|
| **Latest Version** | v5.5.1 (no newer versions available) |
| **Alternative** | `github.com/go-telegram/bot` v1.27.0 (published Sep 2026) ✅ |
| **Migration Required** | ⚠️ **YES - Breaking API changes** |

#### Comparison:

| Feature | Old (`go-telegram-bot-api`) | New (`go-telegram/bot`) |
|---------|---------------------------|------------------------|
| **API Style** | Synchronous methods: `NewBotAPI()`, `GetUpdatesChan()` | Context-based: `NewBot(ctx)`, `GetUpdates(ctx)` |
| **Middleware Support** | ❌ No built-in support | ✅ Full middleware system |
| **Handler Pattern** | Manual update processing | Built-in handler registration |
| **Code Style** | Imperative, procedural | Modern Go with context patterns |

#### Migration Effort Estimate:
- **Lines of Code to Change**: ~50 lines across 3 files
- **Files Affected**:
  - `helpers/ui_helper.go` (~15 lines)
  - `botfixer/bot_fixer.go` (~40 lines)
  - `handlers/tracker.go` (imports only, minimal changes)

#### Example Migration:
```go
// OLD CODE:
type BotFixer struct {
    Bot *tgbotapi.BotAPI
}
func NewBotFixer() *BotFixer {
    b.Bot, err = tgbotapi.NewBotAPI(botFixer.Config.BotAPIKey)
}

// NEW CODE:
type BotFixer struct {
    Bot *bot.Client
}
func NewBotFixer(ctx context.Context) (*BotFixer, error) {
    b, err := bot.New(ctx, bot.Token(botFixer.Config.BotAPIKey))
}
```

#### Recommendation: **MIGRATE** ✅
- **Why**: The new library is actively maintained (Sep 2026), follows modern Go patterns, and provides better architecture for future features.
- **Risk Level**: Medium - requires code refactoring but no business logic changes
- **Time Estimate**: 2-4 hours

---

### 3. Environment Loading (`github.com/joho/godotenv`)  
**Current Version**: `v1.5.1`

#### Usage in Codebase:
```go
// config/config.go
import "github.com/joho/godotenv"
func GetConfig() *Configuration {
    if err := godotenv.Load(); err != nil {
        log.Println("[GetConfig] Error loading .env file")
    }
}
```

#### Analysis:
| Aspect | Details |
|--------|----------|
| **Latest Version** | v1.5.1 (stable, no breaking changes expected) |
| **Alternative** | `github.com/caarlos0/env/v11` or standard library `os.Getenv()` |
| **Migration Required** | ❌ **NO - Optional modernization** |

#### Alternatives:
1. **Keep as-is**: Simple, well-tested, no migration needed
2. **Modernize with `caarlos0/env`**: More features (auto-tagging, validation)
3. **Pure standard library**: Remove dependency entirely

#### Recommendation: **KEEP AS-IS** ✅
- **Why**: Works perfectly for current use case, low risk of issues, minimal value in migrating
- **Risk Level**: Low - only if you want to modernize

---

### 4. Configuration Validation (`github.com/go-playground/validator/v10`)  
**Current Version**: `v10.23.0`

#### Usage in Codebase:
```go
// config/config.go
import "github.com/go-playground/validator/v10"
type Tracker struct {
    Code string `json:"code" validate:"required,excludesall=_/ "`
}
func (c *Configuration) ValidateConfig() {
    validate := validator.New()
    if err := validate.Struct(c); err != nil {
        log.Fatalf("[GetConfig] Config validation error: %v", err)
    }
}
```

#### Analysis:
| Aspect | Details |
|--------|----------|
| **Latest Version** | v10.23.0 (stable) |
| **Alternative** | `github.com/oapi-codegen/runtime` or custom validation |
| **Migration Required** | ❌ **NO - Optional modernization** |

#### Recommendation: **KEEP AS-IS** ✅
- **Why**: Robust, well-maintained, no compelling reason to change
- **Risk Level**: Low - only if you want to reduce dependencies

---

### 5. HTTP Client (`github.com/valyala/fasthttp`)  
**Current Version**: `v1.56.0`

#### Usage in Codebase:
```go
// services/http_service.go
import "github.com/valyala/fasthttp"
func doRequest(url string, requestMethod string) ([]byte, error) {
    req := fasthttp.AcquireRequest()
    resp := fasthttp.AcquireResponse()
}
```

#### Analysis:
| Aspect | Details |
|--------|----------|
| **Latest Version** | v1.56.0 (stable, actively maintained) |
| **Alternative** | Standard library `net/http` or `github.com/gorilla/mux` |
| **Migration Required** | ❌ **NO - Optional modernization** |

#### Recommendation: **KEEP AS-IS** ✅
- **Why**: Fastest HTTP client in Go, excellent for high-performance needs
- **Risk Level**: Low - no migration needed

---

### 6. Web Scraping (`github.com/gocolly/colly/v2`)  
**Current Version**: `v2.1.0`

#### Usage in Codebase:
```go
// clients/scraper_client.go
import "github.com/gocolly/colly/v2"
type ScraperClient struct {
    trackerData *config.Tracker
    collector   *colly.Collector
}
func NewScraperClient() *ScraperClient {
    return &ScraperClient{collector: colly.NewCollector(colly.AllowURLRevisit())}
}
```

#### Analysis:
| Aspect | Details |
|--------|----------|
| **Latest Version** | v2.1.0 (stable) |
| **Alternative** | `github.com/PuerkitoBio/goquery` + manual HTTP, or `github.com/antchfx/htmlquery` |
| **Migration Required** | ❌ **NO - Optional modernization** |

#### Recommendation: **KEEP AS-IS** ✅
- **Why**: Works well for current use case, simple and effective
- **Risk Level**: Low - no migration needed

---

### 7. JSON Parsing (`github.com/tidwall/gjson`)  
**Current Version**: `v1.18.0`

#### Usage in Codebase:
*Note: Currently not directly imported, but used indirectly through config loading*

#### Analysis:
| Aspect | Details |
|--------|----------|
| **Latest Version** | v1.18.0 (stable) |
| **Alternative** | Standard library `encoding/json` |
| **Migration Required** | ❌ **NO - Optional modernization** |

#### Recommendation: **KEEP AS-IS** ✅
- **Why**: Good performance, no compelling reason to change
- **Risk Level**: Low - only if you want to reduce dependencies

---

## Migration Priority Matrix

| Dependency | Priority | Effort | Risk | Recommendation |
|------------|----------|--------|------|----------------|
| Telegram Bot API | 🔴 HIGH | Medium | Medium | ✅ MIGRATE NOW |
| Go Version | 🟢 LOW | None | None | ✅ Already Done |
| godotenv | 🟡 MEDIUM | Low | Low | ⚠️ Optional |
| validator/v10 | 🟡 MEDIUM | Low | Low | ⚠️ Optional |
| fasthttp | 🟢 LOW | None | None | ✅ Keep as-is |
| gocolly/colly | 🟢 LOW | None | None | ✅ Keep as-is |
| tidwall/gjson | 🟡 MEDIUM | Low | Low | ⚠️ Optional |

---

## Detailed Migration Plan: Telegram Bot API

### Phase 1: Preparation (30 minutes)
- [ ] Create backup of current codebase
- [ ] Set up test environment with new library
- [ ] Review new library documentation at https://pkg.go.dev/github.com/go-telegram/bot

### Phase 2: Code Changes (2-4 hours)

#### File 1: `helpers/ui_helper.go`
```go
// BEFORE:
import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
func GetReturnButtonMenu(existingMenu *tgbotapi.InlineKeyboardMarkup) ...

// AFTER:
import (
    "context"
    bot "github.com/go-telegram/bot"
)
type InlineKeyboard struct {
    // adapted for new library
}
```

#### File 2: `botfixer/bot_fixer.go`
```go
// BEFORE:
type BotFixer struct {
    Bot *tgbotapi.BotAPI
}
func NewBotFixer() *BotFixer {
    b.Bot, err = tgbotapi.NewBotAPI(botFixer.Config.BotAPIKey)
}

// AFTER:
type BotFixer struct {
    Bot *bot.Client
}
func NewBotFixer(ctx context.Context) (*BotFixer, error) {
    b, err := bot.New(ctx, bot.Token(botFixer.Config.BotAPIKey))
}
```

#### File 3: `handlers/tracker.go`
```go
// Minimal changes - mostly import updates
import (
    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    // → bot "github.com/go-telegram/bot"
)
```

### Phase 3: Testing (1 hour)
- [ ] Run unit tests for all handler functions
- [ ] Test webhook initialization
- [ ] Test long polling mode
- [ ] Verify message sending/receiving works

### Phase 4: Cleanup (30 minutes)
- [ ] Remove old dependency from go.mod
- [ ] Add new dependency
- [ ] Run `go mod tidy`
- [ ] Update documentation

---

## Total Effort Estimate

| Phase | Time |
|-------|------|
| Preparation | 30 min |
| Code Changes | 2-4 hours |
| Testing | 1 hour |
| Cleanup & Docs | 30 min |
| **Total** | **3.5 - 5.5 hours** |

---

## Risk Assessment

### Telegram Bot API Migration:
| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Breaking changes in new library | Low | High | Test thoroughly before production |
| Code compatibility issues | Medium | Medium | Keep old code as reference during migration |
| Performance regression | Low | Medium | Benchmark after migration |

### Other Dependencies:
- **Low risk** - All alternatives are stable and well-tested
- **No breaking changes expected**

---

## Final Recommendations

### ✅ DO MIGRATE (High Priority):
1. **Telegram Bot API** → `github.com/go-telegram/bot`
   - Modern, actively maintained
   - Better architecture for future features
   - Reasonable migration effort

### ⚠️ CONSIDER MIGRATING (Medium Priority):
2. **godotenv** → Standard library or `caarlos0/env` if you want more features
3. **validator/v10** → Keep as-is unless you have specific needs
4. **tidwall/gjson** → Remove only if not actively used

### ✅ KEEP AS-IS (Low Priority):
5. **fasthttp** - Already optimal for your use case
6. **gocolly/colly** - Works well, no better alternatives needed
7. **Go 1.27** - Already updated!

---

## Next Steps

1. **Review this plan** with the team
2. **Schedule migration window** (recommended: next sprint)
3. **Start with Telegram Bot API migration** as it's the only high-priority item
4. **Document any issues** encountered during migration
5. **Update README.md** with new dependency information after completion

---

## Appendix A: Full Dependency List After Migration

```go
module pricetrackerbot

go 1.27

require (
    github.com/go-telegram/bot v1.27.0      // ← NEW, replaces telegram-bot-api/v5
    github.com/joho/godotenv v1.5.1         // Keep as-is
    github.com/go-playground/validator/v10 v10.23.0  // Keep as-is
    github.com/valyala/fasthttp v1.56.0     // Keep as-is
    github.com/gocolly/colly/v2 v2.1.0      // Keep as-is
)
```

---

## Appendix B: Quick Reference - API Changes

### Telegram Bot API Migration Table:

| Old API | New API |
|---------|----------|
| `tgbotapi.NewBotAPI(token)` | `bot.New(ctx, bot.Token(token))` |
| `b.Bot.GetUpdatesChan(update)` | `b.GetUpdates(ctx)` |
| `tgbotapi.NewMessage(chatID, text)` | `bot.Message(chatID, text)` |
| `tgbotapi.Send()` | `b.Send(ctx, msg)` |
| `tgbotapi.NewInlineKeyboardRow(...)` | `bot.InlineKeyboardRow(...)` |

---

**Document Version**: 1.0  
**Last Updated**: September 23, 2026  
**Author**: AI Assistant
