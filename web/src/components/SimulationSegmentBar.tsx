import { styled, useTheme } from "@mui/material";
import React, { useRef } from "react";

type SimSegProps = {
  critRate: number;
  healthyRate: number;
  maxRate: number;
  onChange(critical: number, healthy: number): void;
};

const SegmentedSlider = styled("div")({
  position: "relative",
  height: "24px",
  width: "100%",
  borderRadius: "4px",
  overflow: "visible",
  cursor: "pointer",
  userSelect: "none",
});

const Segment = styled("div")({
  position: "absolute",
  height: "100%",
});

const Handle = styled("div")({
  position: "absolute",
  top: "50%",
  width: "16px",
  height: "16px",
  borderRadius: "50%",
  transform: "translate(-50%, -50%)",
  backgroundColor: "#fff",
  border: "2px solid #888",
  zIndex: 1,
  cursor: "ew-resize",
  boxShadow: "0 1px 4px rgba(0,0,0,0.3)",
});

export function SimulationSegmentBar({
  critRate,
  healthyRate,
  maxRate,
  onChange,
}: SimSegProps) {
  const theme = useTheme();
  const containerRef = useRef<HTMLDivElement>(null);

  const leftFrac = critRate / maxRate;
  const rightFrac = 1 - healthyRate / maxRate;

  const minGap = 0.02;

  const handlePointerDown = (
    e: React.PointerEvent<HTMLDivElement>,
    handle: "left" | "right",
  ) => {
    e.preventDefault();
    const el = e.currentTarget;
    el.setPointerCapture(e.pointerId);

    const onMove = (moveEvent: PointerEvent) => {
      const rect = containerRef.current?.getBoundingClientRect();
      if (!rect) return;

      const frac = Math.min(
        1,
        Math.max(0, (moveEvent.clientX - rect.left) / rect.width),
      );

      if (handle === "left") {
        const clamped = Math.min(frac, rightFrac - minGap);
        onChange(clamped * maxRate, healthyRate);
      } else {
        const clamped = Math.max(frac, leftFrac + minGap);
        onChange(critRate, (1 - clamped) * maxRate);
      }
    };

    const onUp = () => {
      el.removeEventListener("pointermove", onMove);
      el.removeEventListener("pointerup", onUp);
    };

    el.addEventListener("pointermove", onMove);
    el.addEventListener("pointerup", onUp);
  };

  return (
    <SegmentedSlider ref={containerRef}>
      <Segment
        style={{
          left: 0,
          width: `${leftFrac * 100}%`,
          backgroundColor: theme.palette.error.main,
          borderRadius: "4px 0 0 4px",
        }}
      />
      <Segment
        style={{
          left: `${leftFrac * 100}%`,
          width: `${(rightFrac - leftFrac) * 100}%`,
          backgroundColor: theme.palette.warning.main,
        }}
      />
      <Segment
        style={{
          left: `${rightFrac * 100}%`,
          width: `${(1 - rightFrac) * 100}%`,
          backgroundColor: theme.palette.success.main,
          borderRadius: "0 4px 4px 0",
        }}
      />
      <Handle
        style={{ left: `${leftFrac * 100}%` }}
        onPointerDown={(e) => handlePointerDown(e, "left")}
      />
      <Handle
        style={{ left: `${rightFrac * 100}%` }}
        onPointerDown={(e) => handlePointerDown(e, "right")}
      />
    </SegmentedSlider>
  );
}
