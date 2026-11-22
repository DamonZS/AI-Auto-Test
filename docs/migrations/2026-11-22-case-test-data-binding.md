# Migration: Bind Test Data To Runs

## Summary
Add nullable dataset and row references to cases and runs so a run can use a fixed test-data row without changing existing cases.

## Risk Level
Low. Additive nullable columns only; no existing rows are rewritten or deleted.

## UP
The Go store startup migration adds:

```sql
ALTER TABLE cases ADD COLUMN dataset_id TEXT NOT NULL DEFAULT '';
ALTER TABLE cases ADD COLUMN data_row_id TEXT NOT NULL DEFAULT '';
ALTER TABLE runs ADD COLUMN dataset_id TEXT NOT NULL DEFAULT '';
ALTER TABLE runs ADD COLUMN data_row_id TEXT NOT NULL DEFAULT '';
```

Fresh databases receive the same columns from the schema definition.

## DOWN / Rollback
Stop the service and restore the SQLite file from the pre-deploy backup. The application remains backward-compatible with rows whose new fields are empty. SQLite column removal is intentionally not performed in the same deployment.

## Verification
- Record the database file and row counts before deployment.
- Start the Go service against a copy of the existing database.
- Create a case bound to a dataset row.
- Create a run and verify the run stores the dataset and row IDs.
- Claim the run with an agent token and verify the data row is returned.
- Verify an unrelated tenant receives 404.
