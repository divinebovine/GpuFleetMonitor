import { Card, CardContent, Grid, Typography } from "@mui/material";
import { type GPUHealth } from "../types/gpu";

type FleetSummaryProps = {
  data: GPUHealth[];
};
function FleetSummary({ data }: FleetSummaryProps) {
  const { healthyCount, warningCount, criticalCount } = data.reduce(
    (acc, x) => {
      if (x.status === "healthy") acc.healthyCount++;
      else if (x.status === "warning") acc.warningCount++;
      else if (x.status === "critical") acc.criticalCount++;
      return acc;
    },
    { healthyCount: 0, warningCount: 0, criticalCount: 0 },
  );

  return (
    <>
      <Typography variant="h6" align="center">
        Fleet Summary
      </Typography>
      <Grid
        container
        spacing={6}
        sx={{
          justifyContent: "center",
          alignItems: "center",
        }}
      >
        <Grid size={{ xs: 4, md: 2 }}>
          <Card>
            <CardContent sx={{ textAlign: "center" }}>
              <Typography variant="h4">{healthyCount}</Typography>
              <Typography sx={{ color: "success.main" }}>Healthy</Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid size={{ xs: 4, md: 2 }}>
          <Card>
            <CardContent sx={{ textAlign: "center" }}>
              <Typography variant="h4">{warningCount}</Typography>
              <Typography sx={{ color: "warning.main" }}>Warning</Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid size={{ xs: 4, md: 2 }}>
          <Card>
            <CardContent sx={{ textAlign: "center" }}>
              <Typography variant="h4">{criticalCount}</Typography>
              <Typography sx={{ color: "error.main" }}>Critical</Typography>
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </>
  );
}

export default FleetSummary;
