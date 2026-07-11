import { Card, CardContent, Grid, Typography } from "@mui/material";
import { type GPUHealth } from "../types/gpu";

type FleetSummaryProps = {
  data: GPUHealth[];
};
function FleetSummary({ data }: FleetSummaryProps) {
  const healthyCount = data.filter((x) => x.status === "healthy").length;
  const warningCount = data.filter((x) => x.status === "warning").length;
  const criticalCount = data.filter((x) => x.status === "critical").length;

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
