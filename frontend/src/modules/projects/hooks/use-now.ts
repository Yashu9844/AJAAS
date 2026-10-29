"use client";

import { useEffect, useState } from "react";

/** The current time in ms, refreshed every minute. Reading the clock in render is impure, so it lives in state. */
export function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 60_000);
    return () => clearInterval(id);
  }, []);
  return now;
}
