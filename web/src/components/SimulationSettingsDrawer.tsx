import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Divider,
  Drawer,
  IconButton,
  Slider,
  Typography,
} from "@mui/material";
import { Close } from "@mui/icons-material";
import { useEffect, useState } from "react";
import { useSimulationSettings } from "../hooks/useSimulationSettings";
import { SimulationSegmentBar } from "./SimulationSegmentBar";
import { type SimulationSettings } from "../types/gpu";

const SPEED_PRESETS = [1, 5, 10, 50, 100];
const MAX_RATE = 0.1;
const MAX_OUTCOME_RATE = 0.5;

type Props = {
  open: boolean;
  onClose: () => void;
};

export function SimulationSettingsDrawer({ open, onClose }: Props) {
  const { settings, loading, error, apply, reset } = useSimulationSettings();
  const [draft, setDraft] = useState<SimulationSettings | null>(null);
  const [applying, setApplying] = useState(false);

  useEffect(() => {
    if (settings) setDraft(settings);
  }, [settings]);

  const updateDraft = (patch: Partial<SimulationSettings>) => {
    setDraft((prev) => (prev ? { ...prev, ...patch } : prev));
  };

  const isDirty =
    !!draft && JSON.stringify(draft) !== JSON.stringify(settings);

  const handleApply = async () => {
    if (!draft) return;
    setApplying(true);
    await apply(draft);
    setApplying(false);
  };

  const handleReset = async () => {
    await reset();
  };

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box
        sx={{
          width: 340,
          p: 3,
          display: "flex",
          flexDirection: "column",
          gap: 3,
        }}
      >
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
          }}
        >
          <Typography variant="h6">Simulation Settings</Typography>
          <IconButton onClick={onClose} size="small" aria-label="close">
            <Close />
          </IconButton>
        </Box>

        {loading && (
          <Box sx={{ display: "flex", justifyContent: "center" }}>
            <CircularProgress size={24} />
          </Box>
        )}

        {error && <Alert severity="error">{error}</Alert>}

        {draft && (
          <>
            <Divider />

            <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
              <Typography variant="subtitle2">Simulation Speed</Typography>
              <Box sx={{ display: "flex", gap: 1 }}>
                {SPEED_PRESETS.map((speed) => (
                  <Button
                    key={speed}
                    size="small"
                    variant={
                      draft.speed_multiplier === speed
                        ? "contained"
                        : "outlined"
                    }
                    onClick={() => updateDraft({ speed_multiplier: speed })}
                  >
                    {speed}x
                  </Button>
                ))}
              </Box>
            </Box>

            <Divider />

            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Typography variant="subtitle2">
                Warning State Transitions
              </Typography>
              <Typography variant="caption" color="text.secondary">
                Drag the handles to set how often a GPU in warning transitions
                to critical (red) or recovers to healthy (green). Max rate is{" "}
                {MAX_RATE * 100}%.
              </Typography>
              <SimulationSegmentBar
                critRate={draft.warning_to_critical_rate}
                healthyRate={draft.warning_to_healthy_rate}
                maxRate={MAX_RATE}
                onChange={(crit, healthy) =>
                  updateDraft({
                    warning_to_critical_rate: crit,
                    warning_to_healthy_rate: healthy,
                  })
                }
              />
              <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                <Typography variant="caption" color="error.main">
                  → Critical:{" "}
                  {(draft.warning_to_critical_rate * 100).toFixed(2)}%
                </Typography>
                <Typography variant="caption" color="success.main">
                  → Healthy:{" "}
                  {(draft.warning_to_healthy_rate * 100).toFixed(2)}%
                </Typography>
              </Box>
            </Box>

            <Divider />

            <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
              <Typography variant="subtitle2">
                Healthy → Warning Rate:{" "}
                {(draft.healthy_to_warning_rate * 100).toFixed(2)}%
              </Typography>
              <Slider
                min={0}
                max={MAX_RATE}
                step={0.001}
                value={draft.healthy_to_warning_rate}
                onChange={(_, val) =>
                  updateDraft({ healthy_to_warning_rate: val as number })
                }
                valueLabelDisplay="auto"
                valueLabelFormat={(v) => `${(v * 100).toFixed(2)}%`}
              />
            </Box>

            <Divider />

            <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
              <Typography variant="subtitle2">
                Critical → Warning Rate:{" "}
                {(draft.critical_to_warning_rate * 100).toFixed(2)}%
              </Typography>
              <Typography variant="caption" color="text.secondary">
                Per-tick probability that a thermal or power-capped critical GPU
                steps back to warning on its own. ECC double-bit errors ignore
                this rate entirely.
              </Typography>
              <Slider
                min={0}
                max={MAX_RATE}
                step={0.001}
                value={draft.critical_to_warning_rate}
                onChange={(_, val) =>
                  updateDraft({ critical_to_warning_rate: val as number })
                }
                valueLabelDisplay="auto"
                valueLabelFormat={(v) => `${(v * 100).toFixed(2)}%`}
              />
            </Box>

            <Divider />

            <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
              <Typography variant="subtitle2">Operator Action Outcomes</Typography>
              <Typography variant="caption" color="text.secondary">
                Probability that a GPU lands at Warning instead of Healthy after
                an operator-triggered action.
              </Typography>
              <Typography variant="caption">
                After drain recovery:{" "}
                {(draft.recovery_warning_rate * 100).toFixed(0)}% Warning
              </Typography>
              <Slider
                min={0}
                max={MAX_OUTCOME_RATE}
                step={0.01}
                value={draft.recovery_warning_rate}
                onChange={(_, val) =>
                  updateDraft({ recovery_warning_rate: val as number })
                }
                valueLabelDisplay="auto"
                valueLabelFormat={(v) => `${(v * 100).toFixed(0)}%`}
              />
              <Typography variant="caption">
                After hardware replacement:{" "}
                {(draft.replacement_warning_rate * 100).toFixed(0)}% Warning
              </Typography>
              <Slider
                min={0}
                max={MAX_OUTCOME_RATE}
                step={0.01}
                value={draft.replacement_warning_rate}
                onChange={(_, val) =>
                  updateDraft({ replacement_warning_rate: val as number })
                }
                valueLabelDisplay="auto"
                valueLabelFormat={(v) => `${(v * 100).toFixed(0)}%`}
              />
            </Box>

            <Divider />

            <Box sx={{ display: "flex", gap: 2 }}>
              <Button
                variant="contained"
                onClick={handleApply}
                disabled={applying || !isDirty}
                startIcon={
                  applying ? <CircularProgress size={16} /> : undefined
                }
                fullWidth
              >
                Apply
              </Button>
              <Button
                variant="outlined"
                onClick={handleReset}
                disabled={applying}
                fullWidth
              >
                Reset
              </Button>
            </Box>
          </>
        )}
      </Box>
    </Drawer>
  );
}
