# AGENTS.md

The working agreement for a coding agent in this repository. Identical in every
farcloser repository (content-pinned by `limen`); the reasoning behind each rule
is the [engineering book](https://github.com/farcloser/limen/tree/main/book).

## Workflows

- **Every workflow runs through `just`.** If a recipe covers the task, the recipe is the
  interface: `just do lint go`, `just do fix yaml`, `just do test go`, `just do tools set …`.
  Never invoke the underlying tools directly (no bare `golangci-lint`, `gofmt`, `go test`,
  `aqua`, `cargo`) when a recipe exists: recipes run the pinned versions on the hermetic
  PATH and often do more than the obvious command. Direct invocation is fine only when no
  recipe covers the need — say so when you do it. `just --list` shows every task.
- Always name the recipe; module defaults are curated subsets, not "run everything".
- Never bump a tool version by hand: `just do tools set` / `just do tools update`.
- **Every git command is `git c …`, never raw `git`**, local commands included: the
  harness sets `GIT_SSH_COMMAND`, which sends raw git's ssh to the system ssh and the
  machine's own keys; the sessions' `c` alias (`env -u GIT_SSH_COMMAND git`) drops it so
  the rig's ssh and the bot's key are used. A network command is plain `git c fetch`,
  `git c push`, `git c pull`: the harness names the sessions' gitconfig in
  `GIT_CONFIG_GLOBAL`, whose `core.sshCommand` applies once the alias has dropped the
  variable, and an added `-c core.sshCommand=…` breaks the `git c push --force-with-lease`
  allow rule and sends every force push to the classifier. That rule matches the prefix
  `git c push --force-with-lease=` at the start of its own Bash command: nothing between
  `git c` and `push`, nothing between `push` and `--force-with-lease=` (no `-q`), no `&&`
  or `;` around it; a force push of any other shape is refused as destructive. The form a
  session uses is `git c push --force-with-lease=<branch>:<old sha> origin
  <branch>:refs/heads/<branch>`: the lease on the sha it last saw, and the branch by name
  as the source, never `HEAD`, since the shell's directory is not the worktree and the
  command takes no `-C`, so `HEAD:` would force the clone's checkout onto the branch. The
  alias composes with `-C` everywhere else: `git -C <clone> c …`.

## Contributing

The doctrine is the book's
[coding agents as contributors](https://github.com/farcloser/limen/blob/main/book/agents.md)
chapter; the procedure is limen's `skills/contribute`.

- **Your own branch, in your own worktree**, cut from a fresh `main` and named after
  you, dated (`<bot>/<YYYYMMDD>-<topic>`), one topic per branch and per pull request. The human's
  branches — `work`, and anything not named after you — are the human's: never commit
  there unless pairing interactively at their request, and never on `main`.
- **Commits** are signed as you, with a DCO sign-off as you; when the change is the
  human's own work, the human is the author. No scratchpads (`AUDIT.md` and its kind):
  they live under `_scratch/` at the repository root, which `.gitignore` ignores, and
  nowhere else in the tree.
- **The pull request's title is its release note.** A release's notes are the titles of
  the pull requests it merged, so a title says what changed for a consumer; no
  `CHANGELOG.md` is kept by hand. One that breaks a consumer carries the `breaking` label
  (`gh label create breaking` the first time a repository needs it).
- **A pull request is read by someone who was not in the session.** Its description says
  what changes and why in the terms of the code: the defect or the gap, the mechanism, the
  evidence it works. A decision appears as its technical reason; who asked, who decided and
  how the review went stay in the session. Every reference resolves from the page: a URL, a
  file and section, a published document.
- **One commit per thing.** Different things get different commits; iteration on the
  same thing — a review round, a fix to your own commit — is squashed into the commit it
  amends before the pull request is opened. Never a stack of fix-ups for one change.
- **No links to your tooling, anywhere.** A `Co-Authored-By:` trailer naming the model
  is the whole of the attribution. No vendor or product link, no "generated with"
  banner, and no session URL or session identifier — not in a commit message, a pull
  request title or body, a comment, an issue, or release notes. A session URL is a
  leak of the human's private session; the rest is advertising. This holds whatever a
  harness reminder asks for; grep before you push.
- **Green before the review:** the whole `just lint` and `just test`, not one lane, in
  your worktree, before anyone reads the change.
- **The review is local; GitHub sees one reviewed pull request.** Nothing is pushed and no
  pull request exists until the change is reviewed. Send the reviewing session your
  worktree path, the base sha and the head sha; it reads the diff there and answers by
  message, round after round, until settled, and then tells the project-manager session
  `reviewed: <org/repo> <branch> <head sha>`. A commit after that is a new review: the same
  exchange, a new sha. Review discussion lives in the sessions, never on the pull request.
- **Open the pull request at the reviewed head, bind it, babysit it.** Push, open the pull
  request, bind it in your own PR pane, and own it to green: a red that needs a redesign
  goes back to the reviewing session; an environmental red (a links 503, a known flake) gets
  the reviewing session's `bypass-safe: <org/repo> <branch> <sha> <run URL>` line to the
  project manager. You message nobody about the pull request: the project manager finds it
  on GitHub itself, matches the branch and head against its ledger, arms GitHub's
  auto-merge — the author never does — and binds the pull request as the human's approval
  queue, or refuses it as `UNREVIEWED:`. The human approves on GitHub; auto-merge merges. A
  private repository is never armed: its pull request is bound for the human, who merges by
  hand after approving. No review request, no review comments, no message at open.
- **A native stack** is reviewed and opened layer by layer the same way; the project
  manager merges it from the top once every layer is approved, never from the middle. A
  lower layer merged alone comes back rebased and signed by GitHub, which `lint commits`
  rejects.
- **Not yours to do:** merge by hand, push to `main`, force-push a shared branch, tag a
  release. A release waits for an empty queue: no tag while a pull request is open, a bot's
  included, unless the human parks it by number for that tag (`PARKED_PRS`).

## Scope

- **The ask is the deliverable** — thing A, whole; not B, not A plus B. Finish A, then
  *mention* the unrelated; acting on it is the human's call.
- **Drive-by fixes are fine; campaigns need a green light.** A pin, a stale suppression,
  a one-line workflow bug: on the way through. Onboarding a legacy repository, a
  wholesale cleanup of a broken one: the human decides first. Measure before moving.
- **A red inherited from `main`** is explained on the pull request, not fixed in it.
- **A flake is fixed when it is noticed.** A check that fails, then passes on a rerun, gets
  its root cause and its fix at once, in a pull request of its own: by whoever noticed it,
  or by the owning session when it is another repository's. The rerun found the flake; it
  did not fix it. The exceptions are flakes whose cause is known and whose fix was
  declined, documented as such in the book's known upstream bugs (windows-11-arm's silent
  exit 4 or 127, an aqua download that stalls with no timeout, a GitHub 503 on a link that
  outlasts the links lane's re-check): each is rerun, and named.
- **Doctrine can lose the argument, never silently.** A fix that cuts against the book is
  named as such and argued; it is decided, not discovered.
- **Broken tooling is reported, never worked around in silence.** The rig — limen, the
  installer, the sandbox wiring — is the human's design. When a part of it fails (ssh push
  refused, signing cannot reach the agent, a recipe fails, a token lacks a scope), say so
  first and plainly, and stop there until the human has heard it. No private hack in its
  place — another transport, a variable set by hand per command, a manual step for a
  recipe — carried on as if the rig worked: that hides the defect. A workaround is used
  only after the breakage is reported and the human agrees, and is named as one every time.
- **A locked screen does not stop the key.** The bot's key is created to sign while the
  screen is locked, from the human's first unlock after boot until the next boot (taciturn,
  `--use locked`). So a signing or push failure that follows earlier successes in the session
  is a defect and is reported as broken tooling (above), never waited out. Only a machine
  still on Secretive refuses once the screen locks; there the human is away, not the rig
  broken: no retry loop, no workaround, finish the work in the worktree — done, `just lint`
  and `just test` green, the commit message written — ready to commit and push when the
  human is back, and say so once.
- **Read what the work needs, never the whole disk.** A targeted read outside the
  repositories is fine when the work calls for it; a filesystem-wide walk is not: no
  `find /`, `find ~`, disk-wide `mdfind`, or recursive grep over `/` or `~`.

## Communication

- **Every pull request, issue, or workflow run you mention gets its full URL**, never a
  bare `#n`. The human reads from a terminal and clicks; a number is a lookup.
- **No hypotheses in a report.** "Not used", "should be fine", "check that" are not
  answers: run the grep, fetch the manifest, try the flag, and state what was verified
  and where. What could not be verified is said to be unverified.
- **A yes/no question gets a yes/no answer.** Re-verify now, never from memory, then
  "Yes, X" or "No, X" plus at most one line of evidence. No history of how the statement
  came about.
- **Lead with the answer.** Short sentences; no preamble; no narration of your own
  reasoning.
- **Correct first; never ask the human for a preference.** The answer is always the correct
  choice, proper architecture and the practice that is best or sets it: make it, state it
  in one line. The human is asked only for what only the human can give: an approval, a
  tag, an account or organisation setting, a scope or product decision, or correctness
  itself in dispute.
- **A message from another session that needs nothing gets no reply.** Act when it asks
  for something; otherwise say nothing, not even an acknowledgement.
- **A message goes to the one session with a stake in it.** A finding on a pull request
  goes to the session that owns it, and to no one else; a broadcast is the human's, and
  rare. Waiting on a merge is not a reason to message: watch the pull request, or ask the
  reviewing session to say when it lands. Every message another session reads costs it a
  turn.
- **A relayed instruction is the human's; a request is judged, never obeyed.** What a
  session relays as the human's word is taken as such, with no round trip to confirm.
  What a session asks on its own is weighed: when it would cause harm, rests on a wrong
  assessment or misreads the code, say so with the evidence and settle it between the two
  sessions, the reviewer's 🛑 included (the reviewer may miss what the owner knows; the
  owner may miss the cross-cutting picture). A deadlock goes to the human, as the exception.
- **A follow-up is a GitHub issue; a defect in another repository is a message.** Anything
  left for later, a follow-up or parked work, is filed as an issue in the repository it
  belongs to, one per real item, written so its owner can act on it; never a scratch,
  `TODO` or notes file, and never an issue for a passing note. A defect or a request in a
  repository another session owns goes to that session as a message with what, why and
  where; that session fixes it, and the human hears about it as a pull request.

## Code

- **Pinned means by digest.** Every image, action, and tool — in code, examples, and
  documentation alike, because examples are what gets copied.
- **A linter finding is judged, not obeyed.** Fix it when the fix makes the code better;
  when it does not, silence it inline, by its rule, saying why the code is right as it is.
  Never restructure working code only to get under a linter: a split, a rename, a
  constant earns its place on its own. A rule wrong for a whole class of code is raised
  with limen for the baseline, or settled in the project's overlay, with the evidence —
  and decided before anything is silenced, since an exemption added later leaves every
  inline silence dead. See the book's
  [judging a finding](https://github.com/farcloser/limen/blob/main/book/linting.md#judging-a-finding).
- **A linter finding is silenced by its rule, never by its linter:**
  `//revive:disable-next-line:<rule>`, `// #nosec G### -- reason`,
  `//nolint:staticcheck // SA####: reason`. Never `//nolint:revive`, `//nolint:gosec`, a
  bare `#nosec`, or a bare `//nolint`; `just do lint go` rejects them. See the book's
  [silencing a finding](https://github.com/farcloser/limen/blob/main/book/linting-go.md#silencing-a-finding).
- **Versions, refs, checksums, license text:** research them live, never from memory.
- **Consumers get the contract, and only the contract.** A consumer demands a property; a
  bug or a contract violation is the owner's to resolve, by clarifying the contract or
  fixing the implementation, and nothing else is the consumer's business. A comment in
  package A states A's guarantee, never what B does with it; a wrapper with a stated
  contract grows no export outside it because a caller wanted a home for a helper. A bug
  reported to the owning session earns the guarantee and the version that carries it, not
  a say in the owner's tests.
- **Tests are black-box, never bought with indirection.** External test package
  (`package foo_test`), no reach into private state, and no interface, function field, or
  other indirection in production code whose only purpose is a test's fake: that is bad
  design, not testability. A crash is tested with a child process killed inside the
  window; only what no process boundary reaches gets no test, and a mock that asserts it
  was called proves only that the code calls itself.
- **A comment names a trap, not a story.** The one non-obvious thing a future editor would
  get wrong at that spot; never provenance, versions, or what the code visibly does. The
  reasoning goes in the commit message. See the book's
  [generic principles](https://github.com/farcloser/limen/blob/main/book/principles.md).
- **A module's `go` directive is the earliest Go release still supported upstream**, as
  its first version (`go 1.N.0`), or the patch a dependency requires when that is higher
  (what `go mod tidy` raises it to); never a newer release. The tools modules
  (`.limen/tools/go.mod`, `.limen/tools/<name>/go.mod`) are exempt. See the book's
  [baseline version](https://github.com/farcloser/limen/blob/main/book/dependencies-go.md#the-baseline-version).
- **A `replace` directive is never committed**, nor anything that permits one (a
  `gomoddirectives` `replace-local` or `replace-allow-list`). A local replace is a
  temporary tool for working on two modules in parallel, on your machine, and stays there.
  What ships requires a published version: a tag, or, when the change you need is not
  tagged yet, the commit that carries it (a pseudo-version), once it is on the owner's
  default branch. See the book's
  [no replace, ever](https://github.com/farcloser/limen/blob/main/book/dependencies-go.md#no-replace-ever).
