# Plan: Ollama React UI Improvements

## 1. Model Badge — Colored Circle with Letter + Number

### Design

Each model gets a colored circle badge showing:
- A **letter** derived from the model family (Q for qwen, D for deepseek, etc.)
- A **number** derived from the parameter size (8 for 8b, 30 for 30b, etc.)
- A **background color** assigned by the first 3 characters of the model name

Example renderings:
```
[Q8]   qwen3:8b                    — red circle, white text
[D7]   deepseek-coder:6.7b-instruct — purple circle, white text
[D8]   deepseek-r1:8b              — purple circle, white text
[Q30]  qwen3-coder:30b             — red circle, white text
[N]    nomic-embed-text:latest      — teal circle, white text (no size)
```

### Color Assignment

**Fixed assignments** (first 3 chars of model name, case-insensitive):

| Prefix | Color | Tailwind | Model families |
|--------|-------|----------|----------------|
| `qwe` | Red | `bg-red-600` | qwen3, qwen3-coder |
| `dee` | Purple | `bg-purple-600` | deepseek-coder, deepseek-r1 |

**Dynamic pool** for everything else — assigned in order of first
appearance from a curated swatch list. Colors chosen for high contrast
with white text inside the circle:

```typescript
const COLOR_POOL = [
  { name: "teal",    bg: "bg-teal-600",    text: "text-white" },
  { name: "blue",    bg: "bg-blue-600",    text: "text-white" },
  { name: "amber",   bg: "bg-amber-600",   text: "text-white" },
  { name: "emerald", bg: "bg-emerald-600", text: "text-white" },
  { name: "rose",    bg: "bg-rose-600",    text: "text-white" },
  { name: "indigo",  bg: "bg-indigo-600",  text: "text-white" },
  { name: "orange",  bg: "bg-orange-600",  text: "text-white" },
  { name: "cyan",    bg: "bg-cyan-700",    text: "text-white" },
  { name: "fuchsia", bg: "bg-fuchsia-600", text: "text-white" },
  { name: "lime",    bg: "bg-lime-700",    text: "text-white" },
  { name: "sky",     bg: "bg-sky-600",     text: "text-white" },
  { name: "pink",    bg: "bg-pink-600",    text: "text-white" },
];
```

Assignment is deterministic: hash the 3-char prefix to an index into the
pool, so the same model family always gets the same color across sessions.

### Parsing Logic

```typescript
function parseModelBadge(modelName: string): { letter: string; number: string; color: string } {
  // "deepseek-coder:6.7b-instruct-de1" → prefix="dee", family letter="D"
  const prefix = modelName.slice(0, 3).toLowerCase();
  const letter = modelName[0].toUpperCase();

  // Extract parameter size from name: look for pattern like "6.7b", "8b", "30b"
  // Check after ":" first, then in the model name itself
  const sizeMatch = modelName.match(/(\d+(?:\.\d+)?)[bB]/);
  const number = sizeMatch ? sizeMatch[1] : "";

  const color = getColorForPrefix(prefix);
  return { letter, number, color };
}
```

### Component: `<ModelBadge>`

```tsx
function ModelBadge({ modelName, size = "md" }: { modelName: string; size?: "sm" | "md" }) {
  const { letter, number, colorClasses } = parseModelBadge(modelName);
  const label = number ? `${letter}${number}` : letter;

  const sizeClasses = size === "sm"
    ? "h-5 w-5 text-[9px]"      // sidebar, message headers
    : "h-6 min-w-6 text-[10px]"; // model picker

  return (
    <span className={`${sizeClasses} ${colorClasses} inline-flex items-center
      justify-center rounded-full font-bold leading-none shrink-0`}>
      {label}
    </span>
  );
}
```

### Where to Add It

| Location | File | Current display | Change |
|----------|------|----------------|--------|
| **Model picker button** | `ModelPicker.tsx:183` | Model name text | Add `<ModelBadge>` before name |
| **Model picker dropdown** | `ModelPicker.tsx:337` | Model name text per row | Add `<ModelBadge size="sm">` before name |
| **Message headers** | `Message.tsx` | No model shown | Add `<ModelBadge size="sm">` to assistant messages |
| **Sidebar chat items** | `ChatSidebar.tsx:325` | Chat title only | Optional: add badge if model known |

### New Files

- `src/components/ModelBadge.tsx` — the badge component
- `src/utils/modelBadge.ts` — parsing + color assignment logic

### Files to Modify

- `ModelPicker.tsx` — import and render `<ModelBadge>` in button + dropdown
- `Message.tsx` — add badge to assistant message headers (optional Phase 1)

---

## 2. Narrow Window Layout (300x900, Delphi Panel)

### Problem

The React UI is designed for desktop windows (~800px+ wide). In a narrow
Delphi panel (300px wide, 900px tall), the layout has these issues:

- **Sidebar is open by default** — eats most of the 300px width
- **Font sizes are too large** for the narrow viewport
- **Model picker dropdown** (w-64 = 256px) nearly fills the 300px width
- **Chat input area** has excessive padding
- **Message text** wraps poorly at narrow widths

### Changes

#### A. Auto-hide sidebar on narrow windows

**File:** `src/components/layout/layout.tsx`

Add a `useEffect` that checks window width on mount and on resize.
If width < 800px, auto-close the sidebar:

```tsx
useEffect(() => {
  const checkWidth = () => {
    if (window.innerWidth < 800 && settings.sidebarOpen) {
      setSettings({ SidebarOpen: false });
    }
  };
  checkWidth(); // check on mount
  window.addEventListener("resize", checkWidth);
  return () => window.removeEventListener("resize", checkWidth);
}, []); // run once on mount
```

This respects the user's choice — if they manually open the sidebar on a
narrow window, it stays open. It only auto-closes on initial load /
significant resize.

#### B. Font size scaling for narrow viewports

**File:** `src/index.css`

Add CSS media queries for narrow viewports:

```css
@media (max-width: 480px) {
  /* Reduce base font size for narrow panels */
  html {
    font-size: 14px;  /* default is 16px */
  }
}

@media (max-width: 360px) {
  html {
    font-size: 13px;
  }
}
```

This scales everything proportionally since Tailwind's `text-sm`,
`text-base`, etc. are relative to the root font size when using `rem`.

**Note:** Tailwind uses `rem` units, so changing `html { font-size }`
scales all text proportionally. However, some classes use fixed `px`
values (e.g., `text-[15px]` in ModelPicker). These need individual
overrides or conversion to rem.

#### C. Reduce padding on narrow viewports

**File:** `src/index.css` or component-level changes

```css
@media (max-width: 480px) {
  /* Tighter chat input padding */
  .chat-input-container {
    padding-left: 0.5rem;
    padding-right: 0.5rem;
  }

  /* Narrower message margins */
  .message-container {
    max-width: 100%;
    padding-left: 0.75rem;
    padding-right: 0.75rem;
  }
}
```

Alternatively, add Tailwind responsive prefixes directly in components.
Tailwind's built-in breakpoints are mobile-first (sm: 640px, md: 768px),
so we may need a custom breakpoint or use `max-[480px]:` syntax.

#### D. Model picker width on narrow screens

**File:** `ModelPicker.tsx:204`

Current: `w-64` (256px fixed width).
Change to: `w-64 max-w-[calc(100vw-2rem)]` — caps at viewport width
minus margin.

#### E. Message content width

**File:** `Message.tsx` or relevant container

Currently messages have a `max-w-3xl` (768px) constraint for readability.
On narrow screens this doesn't matter, but the horizontal padding needs
to shrink.

### Files to Modify

| File | Change |
|------|--------|
| `layout/layout.tsx` | Auto-hide sidebar when width < 800px |
| `index.css` | Media queries for font scaling + padding |
| `ModelPicker.tsx` | Max-width on dropdown for narrow screens |
| `ChatForm.tsx` | Reduced padding on narrow screens |
| `Message.tsx` | Reduced message padding on narrow screens |

---

## Implementation Order

1. **`ModelBadge` component** — new file, no risk to existing code
2. **Add badge to ModelPicker** — small change, immediately visible
3. **Auto-hide sidebar** — one `useEffect` in layout.tsx
4. **Font/padding media queries** — CSS only, no component changes
5. **Test at 300x900** in Edge DevTools device emulation
6. **Rebuild SPA** — `cd app/ui/app && npm run build`
7. **Rebuild Go app** — re-embed the new dist/

## Testing

- Open `http://127.0.0.1:3001` in Edge
- Use DevTools → Toggle Device Toolbar → set to 300x900
- Verify sidebar auto-hides
- Verify text is readable
- Verify model picker dropdown fits
- Verify badges render with correct colors
- Test dark mode
