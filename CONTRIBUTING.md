# Contributing

## Branching
- `main` is protected — PR required, 1 approval, CI green.
- Branches: `feat/<scope>`, `fix/<scope>`, `chore/<scope>` (conventional-commits).

## Commits
Conventional Commits: `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`.

## Workflow
```bash
git checkout -b feat/my-feature
# ... work ...
make fmt vet test
git commit -m "feat: add my feature"
gh pr create --fill
```

## Release
Tags `v*.*.*` trigger `release.yaml` → GHCR image + GitHub Release.
