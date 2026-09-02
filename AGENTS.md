# HyperShell Agent Guidance

Read [CLAUDE.md](CLAUDE.md) for the repository structure, commands, and
development conventions.

## Pull Requests

Use a Conventional Commits title for each pull request:

```text
<type>(<optional-scope>)<optional-!>: <description>
```

If the title must contain a Jira key, put the key after the colon:

```text
feat(release): [HYPERSHELL-123] add managed source releases
```

Do not put the Jira key before the conventional type. The repository uses the
pull request title as the squash commit subject. Release Please must be able to
parse the type at the start of that subject. See [docs/releasing.md](docs/releasing.md)
for the allowed types and release rules.
