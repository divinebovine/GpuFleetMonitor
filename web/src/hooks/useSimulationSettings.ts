import { useEffect, useState } from "react";
import { type SimulationSettings } from "../types/gpu";

export function useSimulationSettings() {
  const [settings, setSettings] = useState<SimulationSettings | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchSettings = async () => {
      const resp = await fetch("/api/v1/simulation/settings");
      if (!resp.ok) {
        setError("Unable to fetch simulator settings");
        setLoading(false);
        return;
      }

      try {
        const data = (await resp.json()) as SimulationSettings;
        setSettings(data);
        setLoading(false);
      } catch (err) {
        setError(err instanceof Error ? err.message : "unknown error");
        setLoading(false);
      }
    };

    fetchSettings();
  }, []);

  const apply = async (draft: SimulationSettings) => {
    const resp = await fetch("/api/v1/simulation/settings", {
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      method: "PUT",
      body: JSON.stringify(draft),
    });

    if (!resp.ok) {
      setError(
        "Unable to apply simulation settings, try again in a few minutes.",
      );
      return;
    }

    const applied = (await resp.json()) as SimulationSettings;
    setSettings(applied);
    setError(null);
  };

  const reset = async () => {
    const resp = await fetch("/api/v1/simulation/settings/reset", {
      headers: {
        Accept: "application/json",
      },
      method: "POST",
    });

    if (!resp.ok) {
      setError(
        "Unable to reset simulation settings, try again in a few minutes.",
      );
      return;
    }

    const current = (await resp.json()) as SimulationSettings;
    setSettings(current);
    setError(null);
  };

  return { settings, loading, error, apply, reset };
}
