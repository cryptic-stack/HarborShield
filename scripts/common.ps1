function Get-CurlCommand {
  if (Get-Command "curl.exe" -ErrorAction SilentlyContinue) {
    return "curl.exe"
  }

  if (Get-Command "curl" -ErrorAction SilentlyContinue) {
    return "curl"
  }

  throw "curl was not found on PATH."
}

function Get-NullDevice {
  if ($IsWindows) {
    return "NUL"
  }

  return "/dev/null"
}
