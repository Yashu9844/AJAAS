import type { CalendarToken, CreatedCalendarToken } from "../types/reports";
import { del, getData, post } from "./client";
import { seg } from "./query";

export async function listCalendarTokens(): Promise<CalendarToken[]> {
  return (await getData<CalendarToken[] | null>("/calendar-tokens")) ?? [];
}

export function createCalendarToken(label: string): Promise<CreatedCalendarToken> {
  return post<CreatedCalendarToken>("/calendar-tokens", { label });
}

export function revokeCalendarToken(id: string): Promise<unknown> {
  return del(`/calendar-tokens/${seg(id)}`);
}
