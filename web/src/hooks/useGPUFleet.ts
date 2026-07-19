import { type GPUHealth } from "../types/gpu";
import { useEffect, useState } from "react";

export function useGPUFleet() {
  const [data, setData] = useState<GPUHealth[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [reconnectKey, setReconnectKey] = useState(0);

  useEffect(() => {
    const es = new EventSource("/api/v1/gpus");
    const buffer: GPUHealth[] = [];
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let hasFlushed = false;

    const merge = (prev: GPUHealth[], next: GPUHealth[]): GPUHealth[] => {
      const map = new Map(prev.map((g) => [g.gpu_id, g]));
      for (const g of next) map.set(g.gpu_id, g);
      return Array.from(map.values());
    };

    es.addEventListener("open", () => {
      // fires on initial connect and every reconnect
      buffer.splice(0, buffer.length);
      setLoading(true);
      setError(null);
    });

    es.addEventListener("message", (event) => {
      try {
        buffer.push(JSON.parse(event.data as string) as GPUHealth);
      } catch (err) {
        console.error("failed to parse GPU health event:", err);
      }
    });

    const interval = setInterval(() => {
      if (buffer.length === 0) return;
      const batch = buffer.splice(0, buffer.length);
      setData((prev) => merge(prev, batch));
      setLoading(false);
    }, 200);

    es.addEventListener("done", () => {
      const remaining = buffer.splice(0, buffer.length);
      setData((prev) =>
        merge(prev, remaining).sort((a, b) => a.gpu_id.localeCompare(b.gpu_id)),
      );
      setLoading(false);
      hasFlushed = true;
    });

    es.addEventListener("error", () => {
      console.error("SSE connection error, readyState:", es.readyState);
      if (es.readyState === EventSource.CLOSED) {
        // Non-200 HTTP response (e.g. 502 while the backend is starting after a
        // restart). EventSource does not auto-reconnect on HTTP errors, only on
        // network errors. Schedule a manual reconnect.
        reconnectTimer = setTimeout(() => setReconnectKey((k) => k + 1), 3000);
      } else if (!hasFlushed) {
        setError("stream connection failed");
        setLoading(false);
      }
    });

    return () => {
      clearInterval(interval);
      if (reconnectTimer !== null) clearTimeout(reconnectTimer);
      es.close();
    };
  }, [reconnectKey]);

  return { data, loading, error };
}
