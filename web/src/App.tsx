import "./App.css";
import { useMemo, useState } from "react";
import {
  AppBar,
  Container,
  Toolbar,
  Typography,
  IconButton,
  createTheme,
  ThemeProvider,
  CssBaseline,
  Skeleton,
} from "@mui/material";
import { useGPUFleet } from "./hooks/useGPUFleet";
import GPUList from "./components/GPUList";
import FleetSummary from "./components/FleetSummary";
import { DarkMode, LightMode } from "@mui/icons-material";

const getInitialMode = (): "light" | "dark" => {
  const saved = localStorage.getItem("theme-mode");
  if (saved === "light" || saved === "dark") return saved;
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
};

function App() {
  const [mode, setMode] = useState<"light" | "dark">(getInitialMode);
  const toggleMode = () => {
    const next = mode === "dark" ? "light" : "dark";
    setMode(next);
    localStorage.setItem("theme-mode", next);
  };
  const theme = useMemo(() => createTheme({ palette: { mode } }), [mode]);
  const { data, loading, error } = useGPUFleet();

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <AppBar position="static">
        <Toolbar>
          <Typography variant="h6" align="center" sx={{ flexGrow: 1 }}>
            GPU Fleet Monitor
          </Typography>
          <IconButton
            onClick={toggleMode}
            color="inherit"
            aria-label="Toggle theme"
          >
            {mode === "dark" ? <LightMode /> : <DarkMode />}
          </IconButton>
        </Toolbar>
      </AppBar>
      <Container
        sx={{ pt: 3, display: "flex", flexDirection: "column", gap: 3 }}
      >
        {loading ? (
          <>
            <Skeleton
              variant="rectangular"
              animation="wave"
              height={200}
              width="100%"
            />
            <Skeleton
              variant="rectangular"
              animation="wave"
              height={600}
              width="100%"
            />
          </>
        ) : error ? (
          <Typography color="error">{error}</Typography>
        ) : (
          <>
            <FleetSummary data={data} />
            <GPUList data={data} />
          </>
        )}
      </Container>
    </ThemeProvider>
  );
}

export default App;
