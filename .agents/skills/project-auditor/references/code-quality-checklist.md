# Code Quality Checklist

Standards for assessing code quality, maintainability, and production readiness.

## Type Safety

### TypeScript Projects

| Check | Pass Criteria | How to Verify |
|-------|---------------|---------------|
| Strict mode enabled | `strict: true` in tsconfig.json | Read tsconfig.json |
| No `any` types | No explicit `any` usage | `rg ": any" -g "*.ts"` |
| Null safety | Proper optional chaining | Check for `?.` and `??` usage |
| Return types declared | Functions have explicit return types | Look for `: ReturnType` on functions |
| Props typed | React components have typed props | Check for `interface Props` or `type Props` |

**Red flags:**
```typescript
// BAD: Implicit any
function process(data) { ... }

// BAD: Type assertions to bypass checks
const user = data as any as User;

// BAD: Non-null assertion abuse
const name = user!.profile!.name!;

// GOOD: Proper typing
function process(data: InputData): OutputData { ... }
const name = user?.profile?.name ?? 'Unknown';
```

### Python Projects

| Check | Pass Criteria | How to Verify |
|-------|---------------|---------------|
| Type hints present | Functions have type annotations | Look for `def func(x: Type) -> Type:` |
| Mypy configured | mypy in dependencies + config | Check pyproject.toml |
| Pydantic models | Data classes use Pydantic | Check for `BaseModel` imports |
| Runtime validation | Types enforced at boundaries | Check API handlers |

**Red flags:**
```python
# BAD: No type hints
def process(data):
    return data['value'] * 2

# BAD: Dict instead of model
def create_user(user_data: dict):
    db.insert(user_data)

# GOOD: Proper typing
def process(data: InputModel) -> OutputModel:
    return OutputModel(value=data.value * 2)

def create_user(user_data: UserCreate) -> User:
    validated = UserCreate.model_validate(user_data)
    return db.insert(validated)
```

---

## Error Handling

### Patterns to Check

| Pattern | What to Look For |
|---------|------------------|
| Try-catch coverage | All async operations wrapped |
| Error propagation | Errors bubble up correctly |
| User messages | Generic messages, no internals leaked |
| Logging | Errors logged with context |
| Recovery | Graceful degradation where possible |

**Red flags:**
```python
# BAD: Swallowed exception
try:
    process_payment()
except:
    pass

# BAD: Empty catch
try {
    await fetch(url);
} catch (e) {}

# BAD: Leaked internals
except Exception as e:
    return {"error": str(e), "trace": traceback.format_exc()}

# BAD: Catch-all without re-raise
try:
    something()
except Exception:
    logger.error("Failed")
    # Never re-raises, silently fails
```

**Acceptable patterns:**
```python
# GOOD: Specific exception handling
try:
    result = process_payment(amount)
except PaymentDeclinedError as e:
    logger.warning(f"Payment declined: {e.code}")
    raise HTTPException(400, "Payment was declined")
except PaymentGatewayError as e:
    logger.error(f"Gateway error: {e}", exc_info=True)
    raise HTTPException(503, "Payment system unavailable")

# GOOD: User-friendly message
except Exception:
    logger.exception("Unexpected error in payment processing")
    raise HTTPException(500, "An error occurred. Please try again.")
```

---

## Code Organization

### File Structure

| Check | Pass Criteria |
|-------|---------------|
| Logical grouping | Related code in same directory |
| File size | < 500 lines per file (guideline) |
| Single responsibility | One purpose per module |
| Clear imports | Organized, no circular dependencies |
| Index exports | Clean public API via index files |

### Architecture Patterns

**Backend (Python/Node.js):**
```
src/
├── routers/           # HTTP route handlers
├── services/          # Business logic
├── models/            # Data models
├── schemas/           # Request/response schemas
├── dependencies/      # Dependency injection
├── utils/             # Shared utilities
└── config.py          # Configuration
```

**Frontend (React/Next.js):**
```
src/
├── app/               # Pages (Next.js App Router)
├── components/
│   ├── ui/           # Reusable UI components
│   └── features/     # Feature-specific components
├── hooks/            # Custom hooks
├── lib/              # Utilities and helpers
├── services/         # API client functions
└── types/            # TypeScript types
```

### Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| Files (JS/TS) | camelCase or kebab-case | `userService.ts` or `user-service.ts` |
| Files (Python) | snake_case | `user_service.py` |
| Components | PascalCase | `UserProfile.tsx` |
| Functions | camelCase (JS) / snake_case (Py) | `getUser()` / `get_user()` |
| Constants | UPPER_SNAKE_CASE | `MAX_RETRY_COUNT` |
| Classes | PascalCase | `UserService` |
| Interfaces | PascalCase (no I prefix) | `UserProfile` not `IUserProfile` |

---

## Testing Requirements

### Test Coverage

| Type | Minimum | Ideal |
|------|---------|-------|
| Unit tests | Present | 70%+ coverage |
| Integration tests | Present | Key flows covered |
| E2E tests | Optional | Critical paths covered |

### Test Organization

```
tests/
├── unit/
│   ├── services/
│   └── utils/
├── integration/
│   ├── api/
│   └── database/
├── e2e/
│   └── flows/
├── fixtures/
│   └── sample_data.py
└── conftest.py
```

### Test Quality Checks

| Check | How to Verify |
|-------|---------------|
| Tests exist | Look for test files (`test_*.py`, `*.test.ts`) |
| Tests run | Execute test command, verify pass |
| Mocking used | Check for mock/stub imports |
| Fixtures exist | Look for conftest.py, test utils |
| Coverage config | Check for coverage settings |

**Red flags:**
```python
# BAD: Testing implementation details
def test_user_service():
    service = UserService()
    assert service._internal_cache == {}

# BAD: No assertions
def test_create_user():
    create_user({"name": "Test"})

# BAD: Flaky test (depends on external state)
def test_api():
    response = requests.get("https://external-api.com")
    assert response.status_code == 200
```

**Good patterns:**
```python
# GOOD: Testing behavior
def test_create_user_returns_user_with_id():
    result = create_user(UserCreate(name="Test"))
    assert result.id is not None
    assert result.name == "Test"

# GOOD: Mocking external services
@patch("services.payment.stripe_client")
def test_process_payment(mock_stripe):
    mock_stripe.charge.return_value = ChargeResult(success=True)
    result = process_payment(amount=100)
    assert result.success
```

---

## Documentation

### Required Documentation

| Document | Contents |
|----------|----------|
| README.md | Project overview, setup instructions |
| Environment docs | All env vars documented |
| API docs | OpenAPI/Swagger or equivalent |
| Architecture | High-level system design |

### Code Comments

**When to comment:**
- Complex business logic
- Non-obvious decisions (with rationale)
- Workarounds and their reasons
- Public API docstrings

**When NOT to comment:**
- Obvious code (`i += 1  # increment i`)
- Commented-out code (delete it)
- TODO without owner/timeline

### README Checklist

```markdown
# Project Name

## Overview
Brief description of what this does

## Prerequisites
- Node.js 18+
- PostgreSQL 14+

## Setup
1. Clone repository
2. Install dependencies: `npm install`
3. Copy environment: `cp .env.example .env`
4. Start database: `docker-compose up -d`
5. Run migrations: `npm run migrate`
6. Start server: `npm run dev`

## Environment Variables
| Variable | Description | Required |
|----------|-------------|----------|
| DATABASE_URL | PostgreSQL connection | Yes |
| JWT_SECRET | Token signing key | Yes |

## Testing
```bash
npm run test
npm run test:coverage
```

## Deployment
Deployment instructions or link to docs

## Architecture
Brief architecture overview or link to docs
```

---

## Performance Considerations

### Database

| Check | Why It Matters |
|-------|----------------|
| N+1 queries prevented | Eager loading configured |
| Indexes on foreign keys | Query performance |
| Connection pooling | Resource management |
| Pagination implemented | Memory management |

### API

| Check | Why It Matters |
|-------|----------------|
| Response pagination | Large dataset handling |
| Compression enabled | Bandwidth reduction |
| Caching strategy | Reduce database load |
| Async operations | Non-blocking I/O |

### Frontend

| Check | Why It Matters |
|-------|----------------|
| Bundle size reasonable | Load time |
| Images optimized | Bandwidth |
| Code splitting | Initial load |
| Memoization used | Render performance |

---

## Effort Estimation

### Quick Fix (< 1 hour)
- Adding missing types to a few functions
- Adding error messages
- Fixing naming inconsistencies
- Adding missing env var documentation
- Adding basic README sections

### Moderate (1-4 hours)
- Adding comprehensive type coverage
- Implementing proper error handling pattern
- Adding unit tests for a service
- Refactoring a module to follow patterns
- Setting up linting/formatting

### Significant Refactor (> 4 hours)
- Restructuring project organization
- Implementing proper testing infrastructure
- Adding integration tests
- Refactoring circular dependencies
- Implementing proper dependency injection
