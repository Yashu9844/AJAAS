/** Returns the URL when it is a plain https link without credentials, otherwise null (never render other schemes as links). */
export function safeHttpsUrl(value: string): string | null {
  try {
    const u = new URL(value);
    if (u.protocol !== "https:" || u.username || u.password) return null;
    return u.toString();
  } catch {
    return null;
  }
}
