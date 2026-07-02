"use client";

import { useCallback, useState } from "react";

export function useMessage() {
  const [message, setMessage] = useState("");

  const show = useCallback((value: string) => {
    setMessage(value);
    window.setTimeout(() => setMessage(""), 2600);
  }, []);

  return { message, show };
}
