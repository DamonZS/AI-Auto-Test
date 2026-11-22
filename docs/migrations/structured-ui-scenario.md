# Structured UI scenario contract

UI cases may store a deterministic ordered scenario in the existing versioned `cases.scenario` field. This keeps case revisions, review and run creation on the established control-plane path.

```json
{
  "steps": [
    {"action": "navigate", "url": "https://test.example/"},
    {"action": "fill", "role": "textbox", "name": "Account", "data_key": "account"},
    {"action": "click", "role": "button", "name": "Sign in"},
    {"action": "wait_text", "text": "Welcome"}
  ]
}
```

Supported actions in the migrated main-platform runner are `navigate`, `click`, `fill`, `wait_text`, `assert_text` and `assert_url`. Element actions use the accessible role and name and therefore reuse the platform's Playwright self-healing locator chain. `fill` reads its value by `data_key` from the tenant-bound run data row. The runner never includes that value in step results or failure evidence.

Set `frame` to the iframe's `data-testid` to target a frame. Set `row` to the visible text identifying a single accessible row. Both fields are optional and can be combined. Scoped actions require an exact accessible role and name. Playwright works through open shadow roots with these locators. Use `select` for a native select with its option value in `data_key`, and `check` or `uncheck` for native checkbox state. Apply `frame` and `row` to text assertions when the expected text is scoped.

`wait_text` and `assert_text` are page assertions. A run passes only if at least one assertion ran, every assertion passed and every requested action completed. `assert_url` accepts `contains` (default) or `equals` in `match`. A scenario without an assertion ends as `needs_review`.

The structure is supplied as the case scenario JSON object. Run creation and heartbeat remain unchanged, and the worker receives the versioned scenario with the existing tenant-scoped data row. Natural-language UI scenarios continue through the existing Deep Agents path. API and performance scenarios retain their existing JSON contracts.
