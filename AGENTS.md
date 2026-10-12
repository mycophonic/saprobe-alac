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
- **One commit per thing.** Different things get different commits; iteration on the
  same thing — a review round, a fix to your own commit — is squashed into the commit it
  amends before the review is requested. Never a stack of fix-ups for one change.
- **No links to your tooling, anywhere.** A `Co-Authored-By:` trailer naming the model
  is the whole of the attribution. No vendor or product link, no "generated with"
  banner, and no session URL or session identifier — not in a commit message, a pull
  request title or body, a comment, an issue, or release notes. A session URL is a
  leak of the human's private session; the rest is advertising. This holds whatever a
  harness reminder asks for; grep before you push.
- **Green before pushing:** the whole `just lint` and `just test`, not one lane.
- **Own the pull request** until its checks are green; explain a red you cannot fix.
- **An approved pull request merges itself.** The reviewing session arms GitHub's
  auto-merge with its ✅ at the head it read, and disarms it on a later 🛑 or ⚠️; GitHub
  merges the moment the ruleset is satisfied, the owner's approval plus green checks. The
  author never arms it, never merges by hand, never bypasses. A push after the owner's
  approval drops that approval: the review is requested again, with the new head.
- **Request the owner's review only then** — green and ready. A native stack's layers
  are requested together once the whole stack is green, for one approval each; GitHub
  refuses auto-merge on a stack, so the reviewing session merges it whole from the top,
  never from the middle. A lower layer merged alone comes back rebased and signed by
  GitHub, which `lint commits` rejects. The request is sent once; withdraw it if the pull request
  turns red.
  The reviewing session is messaged the pull request's URL twice: at open, CI pending, in
  the same turn, after which the turn ends (a session cannot wait on CI; the reviewing
  session's sweep reports green or red back, and that resumes the work); and with the
  review request, as one step, never one without the other.
- **Not yours to do:** merge by hand, push to `main`, force-push a shared branch, tag a
  release. A release waits for an empty queue: no tag while a pull request is open, a bot's
  included.

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
- **A locked laptop is not broken tooling.** The bot's key answers only while the human's
  session is unlocked: once the screen locks, the agent refuses to sign or authenticate. So
  when signing or pushing fails after it worked earlier in the session, the human is away,
  not the rig broken. No retry loop, no workaround: finish the work in the worktree — done,
  `just lint` and `just test` green, the commit message written — ready to commit and push
  when the human is back, and say so once. Signing that never worked in the session is
  broken tooling (above).
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
- **No GitHub issues unless the human asks for one.** The issue tracker is the human's.
  A defect or a request that belongs to another repository goes to the session that owns
  that repository, as a message with what, why, and where; that session fixes it, and
  the human hears about it as a pull request. An issue you opened on your own is closed
  once the owning session has the work.

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
  [judging a finding](https://github.com/farcloser/limen/blob/main/book/per-language.md#go--judging-a-finding).
- **A linter finding is silenced by its rule, never by its linter:**
  `//revive:disable-next-line:<rule>`, `// #nosec G### -- reason`,
  `//nolint:staticcheck // SA####: reason`. Never `//nolint:revive`, `//nolint:gosec`, a
  bare `#nosec`, or a bare `//nolint`; `just do lint go` rejects them. See the book's
  [per-language rules](https://github.com/farcloser/limen/blob/main/book/per-language.md).
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
  design, not testability. A property no unit test can observe (a power loss, a
  filesystem failure) gets none; a mock that asserts it was called proves only that the
  code calls itself.
- **A comment names a trap, not a story.** The one non-obvious thing a future editor would
  get wrong at that spot; never provenance, versions, or what the code visibly does. The
  reasoning goes in the commit message. See the book's
  [generic principles](https://github.com/farcloser/limen/blob/main/book/index.md).
- **A module's `go` directive is the earliest Go release still supported upstream**, as
  its first version (`go 1.N.0`), or the patch a dependency requires when that is higher
  (what `go mod tidy` raises it to); never a newer release. The tools modules
  (`tools/go.mod`, `tools/<name>/go.mod`) are exempt. See the book's
  [baseline version](https://github.com/farcloser/limen/blob/main/book/per-language.md#go--the-baseline-version).
- **A `replace` directive is never committed**, nor anything that permits one (a
  `gomoddirectives` `replace-local` or `replace-allow-list`). A local replace is a
  temporary tool for working on two modules in parallel, on your machine, and stays there.
  What ships requires a published version: a tag, or, when the change you need is not
  tagged yet, the commit that carries it (a pseudo-version), once it is on the owner's
  default branch. See the book's
  [no replace, ever](https://github.com/farcloser/limen/blob/main/book/per-language.md#go--no-replace-ever).
