# Vibe-Coded Project Anti-Patterns

Patterns commonly found in AI-generated code that indicate production readiness issues. Based on research showing ~45% of AI-generated code contains security flaws.

## What is Vibe Coding?

**Vibe coding** describes an approach where developers:
1. Describe requirements to an LLM in natural language
2. Accept generated code without thorough review
3. Test primarily through "does it work?" execution
4. Iterate by describing problems back to the LLM

This approach produces functional-looking code that often has:
- Hidden security vulnerabilities
- Incomplete error handling
- Inconsistent patterns
- Over-engineered solutions
- Hallucinated dependencies

---

## Critical Anti-Patterns

### 1. Hallucinated Dependencies

**Problem:** LLMs invent package names that don't exist. Attackers exploit this via "slopsquatting" - creating malicious packages with commonly hallucinated names.

**Detection:**
```bash
# Check if all imports exist
npm ls --all 2>&1 | rg "UNMET"
pip check
```

**Red flags:**
- Import errors on fresh install
- Package names that seem plausible but aren't well-known
- Very recent publish dates on dependencies
- Single-maintainer packages with suspicious code

**Example:**
```python
# AI might generate this - but flask-secure-auth doesn't exist
from flask_secure_auth import require_auth

# The real package is different
from flask_login import login_required
```

**Verification:**
```bash
# Always verify packages exist and are legitimate
npm view <package-name>
pip index versions <package-name>
```

---

### 2. Happy Path Only

**Problem:** AI-generated code often handles the success case but ignores error conditions, edge cases, and failure modes.

**Red flags:**
```javascript
// BAD: No error handling
async function fetchUser(id) {
  const response = await fetch(`/api/users/${id}`);
  const data = await response.json();
  return data;
}

// BAD: Empty catch
try {
  await processPayment();
} catch (e) {
  // TODO: handle error
}
```

**What to look for:**
- Missing try-catch around async operations
- No validation of API response status
- No null/undefined checks
- No timeout handling for external calls
- Missing finally blocks for cleanup

**Fix indicator effort:** MODERATE (systematic review and addition)

---

### 3. Placeholder Security

**Problem:** Code has authentication/authorization structure but no actual enforcement.

**Red flags:**
```python
# BAD: Auth decorator present but does nothing
def require_auth(f):
    @wraps(f)
    def decorated(*args, **kwargs):
        # TODO: implement authentication
        return f(*args, **kwargs)
    return decorated

# BAD: Permission check always passes
def check_permission(user, resource):
    # Will implement RBAC later
    return True

# BAD: Validation that accepts everything
def validate_input(data):
    return data  # No actual validation
```

**What to look for:**
- Decorators with TODO comments
- Permission functions returning `True`
- Empty validation functions
- Auth middleware that calls `next()` unconditionally

**Fix indicator effort:** SIGNIFICANT (requires proper implementation)

---

### 4. Over-Abstraction

**Problem:** AI tends to create unnecessary abstraction layers. Research shows AI code has 2.4x more abstraction layers than human-written equivalents.

**Red flags:**
```python
# BAD: Factory for a factory for a service
class UserServiceFactory:
    def create_service(self) -> UserServiceInterface:
        return UserServiceFactoryImpl().build()

class UserServiceFactoryImpl(AbstractServiceFactory):
    def build(self) -> UserServiceInterface:
        return DefaultUserService(
            repository=UserRepositoryFactory().create(),
            validator=UserValidatorFactory().create(),
            mapper=UserMapperFactory().create(),
        )

# When all you needed was:
class UserService:
    def __init__(self, db: Session):
        self.db = db

    def get_user(self, id: int) -> User:
        return self.db.query(User).get(id)
```

**Indicators:**
- More than 2 levels of inheritance
- Factory patterns for simple objects
- Interface + Implementation for single-use classes
- Dependency injection for trivial dependencies
- Generic type parameters that are never varied

**Fix indicator effort:** MODERATE to SIGNIFICANT (simplification refactor)

---

### 5. Inconsistent Patterns

**Problem:** Different files use different approaches for the same task, suggesting multiple LLM sessions without consistency.

**Examples:**

**Auth patterns varying:**
```python
# File 1: Middleware-based auth
@app.middleware("http")
async def auth_middleware(request, call_next):
    token = request.headers.get("Authorization")
    ...

# File 2: Dependency injection auth
@router.get("/users")
async def get_users(user: User = Depends(get_current_user)):
    ...

# File 3: Manual auth in handler
@router.get("/orders")
async def get_orders(request: Request):
    token = request.headers.get("Authorization")
    user = verify_token(token)
    ...
```

**Error handling varying:**
```typescript
// File 1: Try-catch with specific errors
try {
  await createUser(data);
} catch (error) {
  if (error instanceof ValidationError) {
    return res.status(400).json({ error: error.message });
  }
  throw error;
}

// File 2: Global error handler assumed
await createUser(data); // No try-catch

// File 3: Different error format
try {
  await createUser(data);
} catch (e) {
  return res.status(500).json({ success: false, message: String(e) });
}
```

**Fix indicator effort:** MODERATE (establish and enforce patterns)

---

### 6. Hardcoded Values

**Problem:** AI often hardcodes values that should be configurable.

**Red flags:**
```python
# BAD: Hardcoded URLs
API_URL = "https://api.production.com/v1"

# BAD: Hardcoded credentials (CRITICAL)
DB_PASSWORD = "admin123"

# BAD: Hardcoded limits
MAX_USERS = 100
TIMEOUT = 30

# BAD: Hardcoded feature flags
ENABLE_NEW_FEATURE = True
```

**What should be environment variables:**
- API URLs and endpoints
- Credentials and secrets
- Feature flags
- Limits and thresholds
- Third-party service keys

**Fix indicator effort:** QUICK FIX to MODERATE (extract to config)

---

### 7. Copy-Paste Duplication

**Problem:** AI regenerates similar code when asked for variations instead of creating reusable functions.

**Red flags:**
```python
# BAD: Same logic repeated with minor variations
def create_user(data):
    validated = validate_email(data['email'])
    if not validated:
        raise ValueError("Invalid email")
    user = User(email=data['email'], name=data['name'])
    db.add(user)
    db.commit()
    return user

def create_admin(data):
    validated = validate_email(data['email'])
    if not validated:
        raise ValueError("Invalid email")
    admin = Admin(email=data['email'], name=data['name'], role='admin')
    db.add(admin)
    db.commit()
    return admin

def create_guest(data):
    validated = validate_email(data['email'])
    if not validated:
        raise ValueError("Invalid email")
    guest = Guest(email=data['email'], name=data['name'], role='guest')
    db.add(guest)
    db.commit()
    return guest
```

**Detection:**
- Similar function names with numeric suffixes
- Large blocks of nearly identical code
- Functions that differ by only one line
- Repeated validation logic

**Fix indicator effort:** MODERATE (extract common functionality)

---

### 8. Missing Input Validation

**Problem:** AI often processes user input directly without sanitization.

**Red flags:**
```python
# BAD: Direct database query with user input
@router.get("/search")
def search(query: str):
    return db.execute(f"SELECT * FROM items WHERE name LIKE '%{query}%'")

# BAD: Direct file path from user
@router.get("/files/{path}")
def get_file(path: str):
    return open(path, 'rb').read()

# BAD: Direct URL redirect
@router.get("/redirect")
def redirect(url: str):
    return RedirectResponse(url)

# BAD: User input in template
@router.post("/render")
def render(template: str):
    return jinja2.Template(template).render()
```

**Attack vectors to check:**
- SQL injection (in raw queries)
- Path traversal (file operations)
- Open redirect (URL redirects)
- SSTI (template injection)
- Command injection (shell operations)
- XSS (HTML output)

**Fix indicator effort:** MODERATE to SIGNIFICANT (add validation layer)

---

### 9. Dead Code / Commented Code

**Problem:** AI generates commented-out alternatives or leaves placeholder code.

**Red flags:**
```python
def process_order(order):
    # Old implementation
    # for item in order.items:
    #     process_item(item)

    # New implementation
    # items = order.get_items()
    # for i in items:
    #     pass

    # TODO: implement this
    pass
```

**Indicators:**
- Large commented blocks
- TODO/FIXME/HACK comments without tickets
- Functions that only contain `pass` or `...`
- Unused imports
- Variables assigned but never used

**Fix indicator effort:** QUICK FIX (delete dead code)

---

### 10. Optimistic Assumptions

**Problem:** AI assumes external services always work, data is always valid, and users are always trustworthy.

**Red flags:**
```python
# BAD: Assumes API always returns expected structure
response = requests.get(external_api)
user_data = response.json()['data']['user']['profile']  # No null checks

# BAD: Assumes file always exists
config = json.load(open('config.json'))

# BAD: Assumes database always has data
user = db.query(User).first()
print(user.email)  # Fails if no users

# BAD: Assumes env vars are set
api_key = os.environ['API_KEY']  # Raises KeyError if missing
```

**Defensive alternatives:**
```python
# GOOD: Handle API failures
try:
    response = requests.get(external_api, timeout=10)
    response.raise_for_status()
    user_data = response.json().get('data', {}).get('user', {}).get('profile', {})
except (requests.RequestException, KeyError, json.JSONDecodeError) as e:
    logger.error(f"Failed to fetch user data: {e}")
    user_data = {}

# GOOD: Handle missing env vars
api_key = os.environ.get('API_KEY')
if not api_key:
    raise ConfigurationError("API_KEY environment variable is required")
```

**Fix indicator effort:** MODERATE (add defensive checks throughout)

---

## Detection Checklist

Run these checks on suspected vibe-coded projects:

### Quick Scans
```bash
# Find TODO/FIXME comments
rg "TODO|FIXME|HACK|XXX" -g "*.py" -g "*.ts" -g "*.js"

# Find empty functions
rg -n "^\s*(pass|return)\s*$" -g "*.py"

# Find unused imports (Python)
ruff check --select F401 .

# Find console.log/print statements
rg "console\.log|print\(" -g "*.py" -g "*.ts" -g "*.js"

# Find hardcoded URLs
rg "https?://[a-z0-9]" -g "*.py" -g "*.ts" -g "*.js" -g "!*test*" -g "!node_modules"

# Find potential secrets
rg "(password|secret|key|token)\s*=\s*['\"][^'\"]+['\"]" -g "*.py" -g "*.ts" -g "*.js"
```

### Structural Analysis
```bash
# Count abstraction depth (Python)
rg "class.*:" -g "*.py" --count-matches

# Find factory patterns
rg "Factory" -g "*.py" -g "*.ts"

# Count try-catch ratio vs async operations
rg "try:" -g "*.py" --count-matches
rg "await " -g "*.py" --count-matches
```

---

## Severity by Pattern

| Pattern | Severity | Effort to Fix |
|---------|----------|---------------|
| Hallucinated Dependencies | CRITICAL | Quick (verify/replace) |
| Missing Input Validation | CRITICAL | Moderate |
| Placeholder Security | CRITICAL | Significant |
| Hardcoded Credentials | CRITICAL | Quick |
| Happy Path Only | HIGH | Moderate |
| Optimistic Assumptions | HIGH | Moderate |
| Inconsistent Patterns | MEDIUM | Moderate |
| Over-Abstraction | MEDIUM | Significant |
| Copy-Paste Duplication | MEDIUM | Moderate |
| Dead Code | LOW | Quick |

---

## Remediation Strategy

### Phase 1: Critical Security (Week 1)
1. Verify all dependencies are legitimate
2. Remove hardcoded secrets
3. Implement actual authentication
4. Add input validation on all endpoints

### Phase 2: Stability (Week 2)
1. Add error handling to all async operations
2. Add defensive null checks
3. Implement proper logging
4. Add health check endpoints

### Phase 3: Quality (Week 3-4)
1. Establish consistent patterns
2. Refactor duplicated code
3. Remove dead code
4. Simplify over-abstracted areas

### Phase 4: Polish (Ongoing)
1. Add comprehensive tests
2. Improve documentation
3. Set up CI/CD
4. Configure monitoring
