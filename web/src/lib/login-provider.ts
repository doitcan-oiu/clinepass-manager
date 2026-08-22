export type LoginProvider = "google" | "microsoft"

export function normalizeLoginProvider(raw?: string): LoginProvider {
  const value = (raw || "").trim().toLowerCase()
  if (value === "microsoft" || value === "ms" || value === "outlook" || value === "hotmail" || value === "live") {
    return "microsoft"
  }
  return "google"
}

export function loginProviderLabel(raw?: string) {
  return normalizeLoginProvider(raw) === "microsoft" ? "微软" : "谷歌"
}
