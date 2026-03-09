# Release Notes - 2026-03-09 (Windows Agent Force Self-Update)

## Summary
- Added force-mode support to Windows self-update flow.
- Force mode now supports same-version apply semantics end-to-end.

## Implementation
- `StageIfNeeded`:
  - Reads broadcast/config `mode`.
  - In `force` mode, allows staging even when target version equals current version.
  - Persists `force` into pending metadata.
- `ApplyIfPending`:
  - Uses pending metadata `force` flag.
  - In `force` mode, bypasses same-version no-op guard and triggers helper apply.

## Safety/Idempotency
- Existing download hash verification is unchanged.
- Existing helper-based backup/replace/restart path is unchanged.
- No rollback semantics were removed.

## Validation
- Unit tests updated to cover:
  - force stage on same version
  - force apply on same version
- Live test evidence confirmed staged/apply/helper logs and post-apply hash transition.
