export function formatBytes(value: number) {
  if (!Number.isFinite(value) || value < 0) {
    return "0 B";
  }
  if (value < 1024) {
    return `${value} B`;
  }
  const units = ["KB", "MB", "GB", "TB"];
  let size = value;
  let unitIndex = -1;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  const precision = size >= 10 || unitIndex === 0 ? 1 : 2;
  return `${size.toFixed(precision)} ${units[unitIndex]}`;
}

export function parseHumanBytes(value: string) {
  const trimmed = value.trim();
  if (!trimmed) {
    return null;
  }

  const match = trimmed.match(/^(\d+(?:\.\d+)?)\s*([kmgt]?b)?$/i);
  if (!match) {
    throw new Error("Size values must look like 500 MB, 2 GB, or 1024 B");
  }

  const amount = Number.parseFloat(match[1]);
  if (!Number.isFinite(amount) || amount < 0) {
    throw new Error("Size values must be positive numbers");
  }

  const unit = (match[2] ?? "B").toUpperCase();
  const multiplier =
    unit === "KB" ? 1024 :
    unit === "MB" ? 1024 ** 2 :
    unit === "GB" ? 1024 ** 3 :
    unit === "TB" ? 1024 ** 4 :
    1;

  return Math.round(amount * multiplier);
}
