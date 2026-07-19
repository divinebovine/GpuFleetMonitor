import React, { useState } from "react";
import { TableVirtuoso } from "react-virtuoso";
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow as MuiTableRow,
  Tooltip,
} from "@mui/material";
import { type FailureType, type GPUHealth } from "../types/gpu";

type GPUListProps = {
  data: GPUHealth[];
};

const statusColor = {
  healthy: "success",
  warning: "warning",
  critical: "error",
} as const;

const failureLabel: Record<FailureType, string> = {
  none: "",
  thermal: "Thermal",
  power: "Power Cap",
  ecc_single: "ECC 1-bit",
  ecc_double: "ECC 2-bit",
};

const failureColor: Record<FailureType, "default" | "warning" | "error"> = {
  none: "default",
  thermal: "warning",
  power: "warning",
  ecc_single: "warning",
  ecc_double: "error",
};

async function callRemediation(
  gpuId: string,
  action: "recover" | "replace",
): Promise<{ ok: boolean; message?: string }> {
  const resp = await fetch(`/api/v1/simulation/gpus/${gpuId}/${action}`, {
    method: "PUT",
  });
  if (resp.ok) return { ok: true };
  const text = await resp.text().catch(() => "");
  return { ok: false, message: text || `HTTP ${resp.status}` };
}

function ActionCell({ health }: { health: GPUHealth }) {
  const [loading, setLoading] = useState<"recover" | "replace" | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (health.status === "healthy") return null;

  const canRecover =
    health.failure_type === "thermal" ||
    health.failure_type === "power" ||
    health.failure_type === "ecc_single";

  const handle = async (action: "recover" | "replace") => {
    setLoading(action);
    setError(null);
    const result = await callRemediation(health.gpu_id, action);
    setLoading(null);
    if (!result.ok) setError(result.message ?? "failed");
  };

  const action = canRecover ? "recover" : "replace";
  const color = canRecover ? "success" : "error";
  const label = canRecover ? "Recover" : "Replace";
  const isLoading = loading === action;

  return (
    <Tooltip title={error ?? ""} open={!!error} arrow placement="left">
      <Box sx={{ display: "inline-block" }}>
        <Button
          size="small"
          variant="outlined"
          color={color}
          disabled={!!loading}
          onClick={() => handle(action)}
          sx={{ width: 90, position: "relative" }}
        >
          {/* Keep the label in the DOM so the button width never changes */}
          <span style={{ visibility: isLoading ? "hidden" : "visible" }}>
            {label}
          </span>
          {isLoading && (
            <CircularProgress
              size={14}
              color="inherit"
              sx={{ position: "absolute" }}
            />
          )}
        </Button>
      </Box>
    </Tooltip>
  );
}

const TableComponents = {
  Scroller: React.forwardRef<HTMLDivElement>((props, ref) => (
    <TableContainer component={Paper} {...props} ref={ref} />
  )),
  Table: (props: React.HTMLAttributes<HTMLTableElement>) => (
    <Table {...props} />
  ),
  TableHead: React.forwardRef<HTMLTableSectionElement>((props, ref) => (
    <TableHead {...props} ref={ref} />
  )),
  TableBody: React.forwardRef<HTMLTableSectionElement>((props, ref) => (
    <TableBody {...props} ref={ref} />
  )),
  TableRow: ({ item: _item, ...props }: { item: GPUHealth } & React.HTMLAttributes<HTMLTableRowElement>) => (
    <MuiTableRow
      {...props}
      sx={{ "&:nth-of-type(odd)": { backgroundColor: "action.hover" } }}
    />
  ),
};

function GPUList({ data }: GPUListProps) {
  return (
    <TableVirtuoso
      data={data}
      computeItemKey={(_index, health) => health.gpu_id}
      style={{ height: 600 }}
      overscan={800}
      components={TableComponents}
      fixedHeaderContent={() => (
        <MuiTableRow sx={{ backgroundColor: "background.paper" }}>
          <TableCell sx={{ backgroundColor: "background.paper" }}>
            GPU ID
          </TableCell>
          <TableCell sx={{ backgroundColor: "background.paper" }}>
            Node ID
          </TableCell>
          <TableCell sx={{ backgroundColor: "background.paper" }}>
            Model
          </TableCell>
          <TableCell sx={{ backgroundColor: "background.paper" }}>
            Status
          </TableCell>
          <TableCell sx={{ backgroundColor: "background.paper" }}>
            Failure
          </TableCell>
          <TableCell sx={{ backgroundColor: "background.paper", width: 110 }}>
            Actions
          </TableCell>
        </MuiTableRow>
      )}
      itemContent={(_index, health) => (
        <>
          <TableCell>{health.gpu_id}</TableCell>
          <TableCell>{health.node_id}</TableCell>
          <TableCell>{health.model}</TableCell>
          <TableCell>
            <Chip
              label={health.status}
              color={statusColor[health.status]}
              variant="outlined"
              size="small"
            />
          </TableCell>
          <TableCell>
            {health.failure_type !== "none" && (
              <Chip
                label={failureLabel[health.failure_type]}
                color={failureColor[health.failure_type]}
                variant="filled"
                size="small"
              />
            )}
          </TableCell>
          <TableCell>
            <ActionCell health={health} />
          </TableCell>
        </>
      )}
    />
  );
}

export default GPUList;
