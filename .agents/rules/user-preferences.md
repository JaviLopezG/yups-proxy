______________________________________________________________________

## trigger: always_on

# User preferences (Javi)

How Javi likes things done. Learned over the whole scaffolding of yups; some of
these cost a rework, so take them seriously.

## Communication & process

- Communicate in Spanish; code, identifiers, docs and commit-facing text in
  English.
- He reviews every diff himself and edits freely. ALWAYS re-read current file
  contents before editing; never assume they match what was last written.
- When he questions a choice, give real arguments, not agreement by default —
  but concede quickly when his position is genuinely better.
- He asks WHY constantly, in chat AND documented where the next reader will look
  (README, configuration/). Explain decisions next to the code.

## Conventions

- Markdown code fences must use standard language tags: `bash`, never `console`
  (VS Code syntax highlighting).
- Temp/junk files get extension `.kk` ("caca" phonetic joke): anything created
  with deletion intent carries it; `*.kk` stays gitignored.
- Keep .gitignore minimal (the repo started with a Python template that had to
  be purged).
- Latest distro images over pinned versions, unless reproducibility (a CI
  matrix) actually requires pinning (dind pin is the accepted exception: real
  bug).
- Readable test output matters to him: colours, CamelCase-split test names,
  subtests by leaf name only, summary listing just FAIL/SKIP or "nothing to
  review". He calls these touches "minipoints" and appreciates them.
- NEVER adapt tests to make failing tests pass when the failure reveals a
  program bug: fix the program.
- Security first in CI: no privileged job containers, no docker sockets mounted
  into jobs. Daemon comes from admin-owned dind infra via DOCKER_HOST. Runner
  labels must exist for every image a workflow targets.
- When an edition task is finished commit your changes. If you do different
  tasks in a single turn make a commit for each task.
