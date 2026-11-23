# Update a draft case scenario

`PATCH /api/cases/{id}` updates the scenario content of a case belonging to the request tenant. The endpoint is for editing draft cases only. A successful content edit increments `version_no` and stores the same scenario in `case_revisions` within the same transaction. Reviewed or approved cases must be returned to draft through the existing review workflow before editing.

Request JSON:

```json
{"scenario":"{\"steps\":[{\"action\":\"wait_text\",\"text\":\"Ready\"}]}"}
```

`scenario` is a non-empty string for compatibility with existing API and performance scenario formats. For structured UI cases, its value is the serialized JSON object described in `structured-ui-scenario.md`. The response contains `id`, the new `version_no` and `status: "draft"`. Missing cases return 404, malformed or empty content returns 400, and non-draft cases return 409.

`functional` and `security` must follow the ordered step or fixed checklist shape in [case generation](case-generation.md). Case creation and draft editing return 422 if either shape is invalid.
