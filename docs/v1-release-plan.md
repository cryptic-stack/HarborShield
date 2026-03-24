# v1 Release Plan

This document turns HarborShield's current prerelease proof into a concrete path to a `v1.0.0` release decision.

Current status:

- `v0.1.0-rc4` has a full published-bundle acceptance pass
- install, first-run, core S3, backup, restore, and distributed beta workflow evidence are all captured
- GitHub CI, deep release validation, and tagged publishing are already working

The goal now is not more release plumbing. The goal is a defensible `v1.0` decision with explicit scope, compatibility promises, and a small set of remaining high-confidence closure tasks.

## End Goal

Ship `v1.0.0` only when all of the following are true:

- `single-node` is explicitly defined as the GA scope
- `distributed` remains clearly labeled `beta`
- the supported S3 surface is signed off as the public compatibility contract
- no critical release blockers remain
- every remaining high-severity item is either closed or explicitly accepted in release notes

## Current Position

HarborShield already has:

- published release bundles and pinned image references
- operator evidence from the published `v0.1.0-rc4` asset
- backup and restore evidence from the release path
- distributed beta migration evidence from the release path
- hosted regression coverage for the distributed live-migration lane

That means the path to `v1.0` is now mostly about release decision clarity, not raw feature volume.

## Remaining v1 Decision

The remaining `v1.0` choice is now operational rather than architectural:

- decide whether `v0.1.0-rc4` is the final release candidate
- or cut one more prerelease if you want the signed `v1.0` decision documents bundled into a fresh tagged artifact

## Recommended Path

### Step 1: Decide whether a final prerelease is needed

Use this rule:

- keep `rc4` if the current evidence set is sufficient for your release call
- cut one more `rc` only if you want the freshly signed `v1.0` docs attached to a new tagged artifact

### Step 2: Cut the release

Only cut another prerelease if the scope or compatibility docs change materially enough that you want a fresh tagged artifact to match them.

Otherwise, `rc4` can remain the release-candidate evidence base for the `v1.0` decision.

## Suggested Execution Order

1. decide whether a final `rc` tag is needed
2. cut `v1.0.0`

## Not Required For v1

Do not block `v1.0` on these unless you intentionally expand scope:

- promoting `distributed` from beta to GA
- full distributed recovery automation
- deeper distributed repair or rebalance automation
- broader IAM parity beyond the documented S3 contract
- future storage backends

## Decision Rule

HarborShield is ready for a `v1.0.0` decision when:

- the blocker register shows no open `critical` items
- the GA scope note is published
- the S3 contract is signed off
- the operator-manageability sweep is closed

At that point, success is not “more work happened.” Success is that the release promise is clear, support levels are honest, and the repo contains enough evidence for an external operator to trust the result.
