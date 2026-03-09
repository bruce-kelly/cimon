We just finished a feature or set of changes. Run through this checklist in order. Each step gates the next — do not skip ahead.

## 1. Test verification

Run the full test suite:

```
cd /home/bfk/projects/cimon && export GOROOT=$HOME/go-install/go && export PATH=$PATH:$HOME/go-install/go/bin:$HOME/go/bin && go test ./... -count=1
```

Report the count. If anything fails, fix it before continuing.

Also run the linter:

```
go vet ./...
```

Fix any vet warnings before continuing.

## 2. Workspace hygiene

Run `git status -u` to check for:
- Untracked files that shouldn't exist (backup files, compiled binaries, debug output)
- Unstaged changes that were missed
- Files that should be `.gitignore`d

Clean up anything that doesn't belong before proceeding.

## 3. Bug check

Identify changed files with `git diff --stat` against the last clean commit (or `git log --oneline -15` to find the boundary). Then scan changed files for:
- Logic errors, missing error handling
- Dead code, unused imports (Go compiler catches unused imports, but check for dead functions)
- Broken interactions with existing packages (polling, config, ui, github, db, agents)
- Test count regression — if tests went *down*, investigate

This step requires careful reading — do it yourself, not with a subagent.

## 4. Documentation check

Read and verify these are accurate given the changes:
- **`CLAUDE.md`** — Architecture tree, key patterns, config format, keybindings, theme. Flag anything stale.
- **`README.md`** — Install, usage, config example, screens, keybindings, project structure. Must match reality.

If anything is out of date, fix and commit:
```
git add CLAUDE.md README.md
git commit -m "docs: update docs for [feature name]"
```

## 5. Memory update

Verify `MEMORY.md` (in the auto memory directory) reflects the final state:
- What's been built, what works, what's missing
- Any new patterns or conventions established
- No stale references from previous work

Do NOT commit memory files — they're personal, not repo.

## 6. Push

Push to origin:
```
git push origin main
```

If CI exists, check status:
```
gh run list --branch main --limit 3 --json conclusion,name,status 2>/dev/null || echo "No CI configured yet"
```

Confirm: "Cleanup complete. Tests pass, docs current, pushed."
