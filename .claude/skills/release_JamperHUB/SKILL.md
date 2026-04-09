---
name: release_JamperHUB
description: Create a new JamperHUB release. Commits all changes, creates a git tag, pushes it to trigger GitHub Actions build. Use when the user says "release", "new version", "publish", or "make a release".
argument-hint: "[version, e.g. 0.0.2]"
---

# Release JamperHUB

Create a new release. Argument is the version number (e.g. `0.0.2`).

## Steps

### Step 1: Determine version

If the user provided a version as argument, use it. Otherwise, check the latest tag and increment patch:

```bash
git tag --sort=-v:refname | head -1
```

Ask the user to confirm the version.

### Step 2: Update version in code

Edit `cmd/jamperhub/main.go` -- update the `version` constant:

```go
const version = "X.Y.Z"
```

### Step 3: Commit all changes

```bash
git add -A
git status
```

If there are changes, commit them:

```bash
git commit -m "Release vX.Y.Z"
```

If no changes (only version bump), commit just the version:

```bash
git commit -m "Bump version to X.Y.Z"
```

### Step 4: Delete old tag if same version exists

```bash
gh release delete vX.Y.Z --repo eduard256/JamperHUB --yes 2>/dev/null
git push origin :refs/tags/vX.Y.Z 2>/dev/null
git tag -d vX.Y.Z 2>/dev/null
```

### Step 5: Tag and push

```bash
git push origin main
git tag vX.Y.Z
git push origin vX.Y.Z
```

### Step 6: Verify

Wait 5 seconds, then check that Actions started:

```bash
gh run list --repo eduard256/JamperHUB --limit 1
```

Tell the user:
- Tag `vX.Y.Z` created and pushed
- GitHub Actions building: link to the run
- Release will appear at `https://github.com/eduard256/JamperHUB/releases/tag/vX.Y.Z`
