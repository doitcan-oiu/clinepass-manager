export function emailSuffix(email: string) {
  const value = email.trim().toLowerCase()
  const at = value.lastIndexOf("@")
  if (at < 0 || at === value.length - 1) return ""
  return value.slice(at + 1).replace(/^\.+|\.+$/g, "")
}

export function parseBatchAccountLines(text: string) {
  return text.split(/\r?\n/).flatMap((raw, index) => {
    const line = raw.trim()
    if (!line || line.startsWith("#")) return []
    const email = line.split("----")[0]?.trim() || ""
    if (!email) return []
    return [{ index, line: raw, email, suffix: emailSuffix(email) }]
  })
}

export function blacklistHits(text: string, blacklist: string[]) {
  const blocked = new Set(blacklist.map((s) => s.trim().toLowerCase().replace(/^@/, "")).filter(Boolean))
  return parseBatchAccountLines(text).filter((row) => row.suffix && blocked.has(row.suffix))
}

export function stripBlacklistedLines(text: string, blacklist: string[]) {
  const blocked = new Set(blacklistHits(text, blacklist).map((row) => row.index))
  return text
    .split(/\r?\n/)
    .filter((_, index) => !blocked.has(index))
    .join("\n")
}
