import React from "react";
import { TableVirtuoso } from "react-virtuoso";
import {
  Chip,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow as MuiTableRow,
} from "@mui/material";
import { type GPUHealth } from "../types/gpu";

type GPUListProps = {
  data: GPUHealth[];
};

const statusColor = {
  healthy: "success",
  warning: "warning",
  critical: "error",
} as const;

function GPUList({ data }: GPUListProps) {
  return (
    <TableVirtuoso
      data={data}
      style={{ height: 600 }}
      overscan={800}
      components={{
        Scroller: React.forwardRef((props, ref) => (
          <TableContainer component={Paper} {...props} ref={ref} />
        )),
        Table: (props) => <Table {...props} />,
        TableHead: React.forwardRef((props, ref) => (
          <TableHead {...props} ref={ref} />
        )),
        TableBody: React.forwardRef((props, ref) => (
          <TableBody {...props} ref={ref} />
        )),
        TableRow: ({ item: _item, ...props }) => (
          <MuiTableRow
            {...props}
            sx={{
              "&:nth-of-type(odd)": { backgroundColor: "action.hover" },
            }}
          />
        ),
      }}
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
            />
          </TableCell>
        </>
      )}
    />
  );
}

export default GPUList;
