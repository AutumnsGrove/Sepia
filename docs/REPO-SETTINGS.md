# Repository settings

The settings that are not in the code, because GitHub keeps them, and the one thing S00 cannot do for me. The
rule they implement is in [`CI.md`](CI.md) 4.6 and the reasoning is in [`BUILD.md`](BUILD.md) 5.

## Now, and after S13: direct pushes to `main`

A single-operator repository pushing straight to `main` cannot also require a pull request, so the guard rails
move onto the push. Apply these once:

- **Branch ruleset on `main`**
  - Target: the default branch.
  - **Block force pushes** and **block deletions**: on.
  - **Require linear history**: on.
  - **Require status checks to pass before merging**: on, and add the required checks below. Because direct
    pushes are allowed, the checks are reported on the push itself; a red run is visible within the run.
  - **Require a pull request**: **off**, now and after S13.
- **Required status checks:** `go`, `nocgo`, `coverage`, `drift`, `web`, `e2e`, `guard`, `deps`, `codeql`. `deps` only
  runs on pull requests, so it is listed but not enforced; it is advisory for the pull requests Autumn opens by
  preference.
- **Secret scanning** and **push protection**: on. The repo is public and holds no keys, and push protection
  is what stops the first one being added by accident.
- **Dependabot alerts** and **security updates**: on.
- **Actions**: allow only the actions in `.github/workflows/` (GitHub-owned, plus `golangci`, `gitleaks`,
  `pnpm/action-setup`). Default workflow permissions: read. Turn on **require actions to be pinned by SHA** if
  the setting is offered: every action here is pinned, and this makes the check enforcement rather than habit.
- **Environments**: create `evals` and `live-checks`, each with **Autumn** as a required reviewer. Every
  secret that is not a reviewer credential lives here, and neither environment is triggered by a pull request
  ([`CI.md`](CI.md) 3).
- **Reviewer credentials** are the one repository-level exception, named in [`CI.md`](CI.md) 4.5, because a
  `pull_request` review job cannot wait on an Environment's required reviewer.

## From S13: no change

The phase this file used to describe (pull requests required, one approving review, `deps` enforced) was
**declined on 2026-11-14**. Autumn pushes to `main` occasionally and mostly works through pull requests by
preference, so requiring a review on a single-operator repository adds ceremony and no reviewer. The settings
above stay exactly as they are, including `deps` as advisory. Nothing here changes at deploy and update.

## Never

- No self-hosted runner. A public repository plus a self-hosted runner lets any fork run code on that machine.
- No `pull_request_target`, and no workflow that checks out or runs fork code with secrets.
- No repository-level secrets, except the reviewer credentials [`CI.md`](CI.md) 4.5 names. Every other key
  lives in `evals` or `live-checks`, and neither of those is reachable from a pull request.
- No Tailscale Funnel, no `0.0.0.0` bind, and nothing that puts the journal on a public interface.
