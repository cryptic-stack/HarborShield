# First-Run Walkthrough Checklist

This checklist turns the first-run guide into a release-quality operator acceptance pass.

Use it when validating:

- a clean install from empty Docker volumes
- the first administrator experience
- the first deployment decision
- the first object workflow

Primary reference:

- [`docs/first-run.md`](c:\Users\JBrown\Documents\Project\s3-platform\docs\first-run.md)

Automated companion:

- [`scripts/release-clean-install-smoke.ps1`](c:\Users\JBrown\Documents\Project\s3-platform\scripts\release-clean-install-smoke.ps1)

## Preconditions

- `.env` exists and required secrets were rotated
- Docker Desktop is running
- no old HarborShield volumes are in use for this walkthrough
- stack is started with:

```powershell
docker compose --env-file .env up --build -d
```

Expected baseline:

- `docker compose ps` shows all core services healthy
- `http://localhost/healthz` returns `{"status":"ok"}`

## Walkthrough Steps

### 1. Open the platform

Action:

- open `http://localhost`

Expected:

- login screen renders fully
- auth form text is readable
- error messaging is professional and human-readable

Release notes:

- block release if the page is blank, crashes after paint, or input text is unreadable

### 2. Bootstrap login

Action:

- sign in with `ADMIN_BOOTSTRAP_EMAIL`
- use `ADMIN_BOOTSTRAP_PASSWORD`

Expected:

- login succeeds
- no raw JSON error blobs appear
- operator is redirected into the password change flow

Release notes:

- block release if invalid styling, broken redirects, or stale-session loops occur on a clean install

### 3. Forced password change

Action:

- enter a new strong password

Expected:

- change succeeds with clear success feedback
- old bootstrap password no longer works
- new password works immediately

Release notes:

- block release if the operator can bypass password change on first login

### 4. First-run deployment wizard

Action:

- complete the setup wizard

Expected:

- the operator can choose:
  - `Single Node`
  - `Distributed`
- if `Distributed`, the operator can choose:
  - `Local Nodes`
  - `Remote Nodes`
- if `Remote Nodes`, endpoint input is clear and validated
- if runtime changes are still needed, the UI stays on the wizard and shows an exact apply step

Release notes:

- block release if the wizard is confusing, missing required branching, or gives no clear next action

### 5. Settings review

Action:

- open `Settings`

Expected:

- storage mode is understandable
- OIDC section is readable and clearly explains status
- no blank sections or raw config values leak into the UI

Release notes:

- log issues if labels are inconsistent, values are unclear, or secret handling is ambiguous

### 6. Bucket creation

Action:

- open `Buckets`
- create the first bucket

Expected:

- create action gives success or clear validation feedback
- empty-state pages do not crash
- storage class and durability inputs are understandable

Release notes:

- block release if create actions appear dead or silently fail

### 7. Credential creation

Action:

- open `Credentials`
- create an S3 credential

Expected:

- access key and secret are shown once
- secret handling is clearly explained
- created credential appears in the list with role and last-used visibility

### 8. First object workflow

Action:

- open `Uploads`
- upload one or more files
- download a file back
- delete a file

Expected:

- long filenames do not break layout
- multi-upload queue is readable
- file sizes are human-readable
- delete and download actions do not throw raw parse or JSON errors

Release notes:

- block release if uploads crash, queue statuses are confusing, or object actions return raw backend payloads

### 9. Audit confirmation

Action:

- open `Audit`

Expected:

- recent login, settings, bucket, credential, and object actions are visible
- categories and severity badges are readable
- settings changes show masked, diff-style audit output

### 10. Optional OIDC setup pass

Action:

- open `Settings`
- save OIDC values
- run `Test Connection`
- clear the stored secret

Expected:

- save works without exposing the secret back to the UI
- connection test returns a readable result
- clearing the secret updates state correctly

## Common Friction Points To Review

- auth page readability on dark theme
- capitalization and professionalism of error messages
- empty-state handling on fresh installs
- success feedback after create, save, upload, and delete actions
- setup wizard clarity around distributed local vs distributed remote
- OIDC form clarity after first login

## Release Gate For UX-001

Mark `UX-001` complete only if:

- the full walkthrough succeeds on a clean install
- no blank pages appear during first-run admin use
- no raw JSON payloads are shown to the operator as error text
- bootstrap login, password change, setup wizard, first bucket, first credential, first upload, and audit review all complete without guesswork

## Follow-On Fix Log

Use this short format while closing remaining first-run issues:

| ID | Area | Problem | Severity | Status |
| --- | --- | --- | --- | --- |
| UX-001-A | Auth | Example: low-contrast auth helper text | Medium | Open |
| UX-001-B | Setup | Example: remote endpoint validation unclear | Medium | Open |
| UX-001-C | Uploads | Example: delete success feedback too subtle | Low | Open |
