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

- execute the final `v1.0.0` tag and publish flow successfully

## Recommended Path

### Step 1: Cut the release

Use the existing `rc4` runtime evidence plus the signed `v1.0` documents as the basis for the final GA tag.

## Suggested Execution Order

1. cut `v1.0.0`

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
