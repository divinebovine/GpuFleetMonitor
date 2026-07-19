# 002 — react-virtuoso TableVirtuoso Over @virtuoso.dev/data-table

## Status
Accepted

## Context
The GPU list renders 10,000 rows and required virtualization to keep the DOM manageable. Two options from the same library author were evaluated:

- `@virtuoso.dev/data-table` — the newer, column-oriented API with a headless and shadcn distribution
- `react-virtuoso` `TableVirtuoso` — the predecessor, renders real `<table>` elements

The project uses MUI for all UI components. `@virtuoso.dev/data-table` was installed and partially implemented first.

## Decision
Switched to `react-virtuoso`'s `TableVirtuoso` after discovering that `@virtuoso.dev/data-table`'s headless distribution is designed for Tailwind/shadcn codebases. The shadcn wrapper provides styled cells and headers out of the box, but requires Tailwind. The headless version ships only structural CSS (`data-table-element-role` flex attributes) and no visual styling, requiring significant custom CSS work to achieve what MUI's table components provide for free.

`TableVirtuoso` accepts MUI components directly via its `components` prop (`Table`, `TableHead`, `TableBody`, `TableRow`, `TableContainer`), making the migration from a plain MUI table minimal and keeping all MUI theming intact.

## Consequences
- Table looks and behaves identically to the pre-virtualization MUI table
- Light/dark theme continues to work via MUI's ThemeProvider without additional CSS
- `forwardRef` wrappers required on `Scroller`, `TableHead`, and `TableBody` so TableVirtuoso can measure scroll position and dimensions
- Lost access to `@virtuoso.dev/data-table` features: column resizing, reordering, visibility toggles, and state persistence — these would require additional implementation if needed later
- `@virtuoso.dev/data-table` may be worth revisiting if the project adopts Tailwind or shadcn/ui
