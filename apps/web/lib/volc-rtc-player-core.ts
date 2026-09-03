export type VolcRTCPlaybackCredential = {
  appId: string;
  roomId: string;
  token: string;
  userId: string;
};

export function parseVolcRTCPlaybackCredential(raw: string): VolcRTCPlaybackCredential {
  if (!raw || raw.length > 16_384 || raw !== raw.trim() || /[\0\r\n]/.test(raw)) {
    throw new Error("VOLC_RTC_CREDENTIAL_INVALID");
  }
  let values: Record<string, unknown>;
  if (raw.startsWith("{")) {
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("VOLC_RTC_CREDENTIAL_INVALID");
    values = parsed as Record<string, unknown>;
  } else {
    values = Object.fromEntries(new URLSearchParams(raw));
  }
  const required = ["app_id", "room_id", "token", "user_id"] as const;
  const result = Object.fromEntries(required.map((key) => {
    const value = values[key];
    if (typeof value !== "string" || value.length < 1 || value.length > 4096 || /[\0\r\n]/.test(value)) {
      throw new Error("VOLC_RTC_CREDENTIAL_INVALID");
    }
    return [key, value];
  })) as Record<(typeof required)[number], string>;
  return { appId: result.app_id, roomId: result.room_id, token: result.token, userId: result.user_id };
}
