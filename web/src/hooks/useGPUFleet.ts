import { type GPUHealth } from "../types/gpu";
import { useEffect, useState } from "react";

export function useGPUFleet() {
  const [data, setData] = useState<GPUHealth[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const es = new EventSource("/api/v1/gpus");
    const buffer: GPUHealth[] = [];

    es.addEventListener("message", (event) => {
      buffer.push(JSON.parse(event.data as string) as GPUHealth);
    });

    const interval = setInterval(() => {
      if (buffer.length === 0) return;
      const batch = buffer.splice(0, buffer.length);
      setData((prev) => [...prev, ...batch]);
      setLoading(false);
    }, 200);

    es.addEventListener("done", () => {
      clearInterval(interval);
      const remaining = buffer.splice(0, buffer.length);
      setData((prev) =>
        [...prev, ...remaining].sort((a, b) =>
          a.gpu_id.localeCompare(b.gpu_id),
        ),
      );
      setLoading(false);
      es.close();
    });

    es.onerror = () => {
      clearInterval(interval);
      setError("stream connection failed");
      setLoading(false);
      es.close();
    };

    return () => {
      clearInterval(interval);
      es.close();
    };
  }, []);

  return { data, loading, error };
}
