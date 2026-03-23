param(
  [string]$Email,
  [string]$Password
)

$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $projectRoot ".env"

if (-not (Test-Path $envFile)) {
  throw ".env not found at $envFile"
}

$envValues = @{}
Get-Content $envFile | ForEach-Object {
  $line = $_.Trim()
  if (-not $line -or $line.StartsWith("#")) {
    return
  }
  $parts = $line -split "=", 2
  if ($parts.Count -eq 2) {
    $envValues[$parts[0]] = $parts[1]
  }
}

if (-not $Email) {
  $Email = $envValues["ADMIN_BOOTSTRAP_EMAIL"]
}
if (-not $Password) {
  $Password = $envValues["ADMIN_BOOTSTRAP_PASSWORD"]
}

if (-not $Email -or -not $Password) {
  throw "ADMIN_BOOTSTRAP_EMAIL and ADMIN_BOOTSTRAP_PASSWORD must be set in .env or passed explicitly."
}

$postgresUser = $envValues["POSTGRES_USER"]
$postgresDb = $envValues["POSTGRES_DB"]
if (-not $postgresUser -or -not $postgresDb) {
  throw "POSTGRES_USER and POSTGRES_DB must be set in .env."
}

$safeEmail = $Email.Replace("'", "''")
$safePassword = $Password.Replace("'", "''")

$sql = @"
INSERT INTO users (email, password_hash, role, must_change_password, auth_provider, external_subject)
VALUES ('$safeEmail', crypt('$safePassword', gen_salt('bf')), 'superadmin', TRUE, 'local', '')
ON CONFLICT (email) DO UPDATE
SET password_hash = crypt('$safePassword', gen_salt('bf')),
    role = 'superadmin',
    must_change_password = TRUE,
    auth_provider = 'local',
    external_subject = '',
    updated_at = NOW();

DELETE FROM refresh_tokens
WHERE user_id IN (SELECT id FROM users WHERE email = '$safeEmail');
"@

docker compose exec -T postgres psql -U $postgresUser -d $postgresDb -v ON_ERROR_STOP=1 -c $sql | Out-Host

Write-Host "Bootstrap admin reset complete for $Email"
Write-Host "Next login will require a password change."
