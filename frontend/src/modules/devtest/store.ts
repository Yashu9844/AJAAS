"use client";
import { useSyncExternalStore } from "react";
import type { Exchange, Session, Slot, StoredResult } from "./types";

export interface State {
  host: string;
  port: string;
  sessions: { A: Session | null; B: Session | null };
  ids: Record<string, string>;
  results: Record<string, StoredResult>;
  history: Exchange[];
}

const INITIAL: State = {
  host: "localhost",
  port: "8080",
  sessions: { A: null, B: null },
  ids: {},
  results: {},
  history: [],
};

const LS = "jaas-devtest-v1";
const SS = "jaas-devtest-sessions-v1";
let state: State = INITIAL;
let loaded = false;
const listeners = new Set<() => void>();

function load() {
  if (loaded || typeof window === "undefined") return;
  loaded = true;
  try {
    const l = JSON.parse(localStorage.getItem(LS) ?? "null");
    if (l) {
      state = { ...state, host: l.host ?? state.host, port: l.port ?? state.port, ids: l.ids ?? {}, results: l.results ?? {}, history: l.history ?? [] };
    }
  } catch {}
  try {
    const s = JSON.parse(sessionStorage.getItem(SS) ?? "null");
    if (s) state = { ...state, sessions: { A: s.A ?? null, B: s.B ?? null } };
  } catch {}
}

function persist() {
  try {
    localStorage.setItem(
      LS,
      JSON.stringify({ host: state.host, port: state.port, ids: state.ids, results: state.results, history: state.history.slice(0, 40) }),
    );
    sessionStorage.setItem(SS, JSON.stringify(state.sessions));
  } catch {}
}

function emit() {
  persist();
  listeners.forEach((l) => l());
}

export function getState(): State {
  load();
  return state;
}

function subscribe(cb: () => void) {
  load();
  listeners.add(cb);
  return () => listeners.delete(cb);
}

export function useDevStore(): State {
  return useSyncExternalStore(subscribe, getState, () => INITIAL);
}

export function setConfig(host: string, port: string) {
  state = { ...state, host, port };
  emit();
}
export function setSession(slot: Slot, s: Session | null) {
  state = { ...state, sessions: { ...state.sessions, [slot]: s } };
  emit();
}
export function patchSession(slot: Slot, patch: Partial<Session>) {
  const cur = state.sessions[slot];
  if (!cur) return;
  setSession(slot, { ...cur, ...patch });
}
export function setId(key: string, value: string) {
  state = { ...state, ids: { ...state.ids, [key]: value } };
  emit();
}
export function setResult(key: string, r: Omit<StoredResult, "at">) {
  state = { ...state, results: { ...state.results, [key]: { ...r, at: Date.now() } } };
  emit();
}
export function clearResults() {
  state = { ...state, results: {} };
  emit();
}
export function pushExchange(ex: Exchange) {
  state = { ...state, history: [ex, ...state.history].slice(0, 100) };
  emit();
}
export function clearHistory() {
  state = { ...state, history: [] };
  emit();
}
