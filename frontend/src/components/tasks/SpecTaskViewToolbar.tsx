import React, { ReactNode, useState } from "react";
import {
  Box,
  CircularProgress,
  IconButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  CloudUpload,
  EllipsisVertical,
  Files,
  Globe2,
  Lock,
  LockOpen,
  LucideIcon,
  MessageSquare,
  MonitorPlay,
  PanelBottom,
  PanelLeft,
  PanelRight,
  GitCompare,
  Play,
  RotateCw,
  SlidersHorizontal,
  Square,
  X,
} from "lucide-react";
import { useElementWidth } from "../../hooks/useElementWidth";
import useIsPhone from "../../hooks/useIsPhone";

export type TaskView =
  | "chat"
  | "desktop"
  | "browser"
  | "changes"
  | "files"
  | "details";

/**
 * How much room the toolbar has. Derived from the toolbar's own measured width
 * (not the viewport) because in split view the toolbar only occupies the right
 * panel — a wide window can still leave it very narrow.
 */
export type ToolbarDensity = "comfortable" | "compact" | "tight";

const COMPACT_BELOW = 660;
const TIGHT_BELOW = 460;

export function toolbarDensityForWidth(width: number): ToolbarDensity {
  // Before the first measurement assume the roomiest layout; wrapping covers
  // the single frame before the ResizeObserver reports.
  if (width === 0) return "comfortable";
  if (width < TIGHT_BELOW) return "tight";
  if (width < COMPACT_BELOW) return "compact";
  return "comfortable";
}

const TAB_METRICS: Record<
  ToolbarDensity,
  { width: number; height: number; icon: number; showLabel: boolean }
> = {
  comfortable: { width: 56, height: 40, icon: 18, showLabel: true },
  compact: { width: 46, height: 36, icon: 16, showLabel: true },
  tight: { width: 32, height: 32, icon: 16, showLabel: false },
};

const ICON_BUTTON_METRICS: Record<
  ToolbarDensity,
  { size: number; icon: number }
> = {
  comfortable: { size: 30, icon: 18 },
  compact: { size: 28, icon: 16 },
  tight: { size: 28, icon: 16 },
};

export function toolbarIconButtonSx(density: ToolbarDensity) {
  const { size, icon } = ICON_BUTTON_METRICS[density];
  return {
    width: size,
    height: size,
    minWidth: size,
    minHeight: size,
    p: 0.5,
    flexShrink: 0,
    color: "text.secondary",
    "& svg": { width: icon, height: icon },
    "&:hover": {
      color: "text.primary",
      backgroundColor: "action.hover",
    },
    '&[aria-pressed="true"]': {
      backgroundColor: "action.selected",
    },
  } as const;
}

interface ViewTab {
  value: TaskView;
  label: string;
  icon: LucideIcon;
  /** Only meaningful while the task has a session (desktop, diff, files, …). */
  sessionOnly: boolean;
  /** Chat is a tab only when chat has no panel of its own. */
  chatOnly?: boolean;
  /**
   * Folded into the overflow menu on a phone. Six tabs plus the lifecycle
   * controls do not fit across 390px, and these three are the ones you visit
   * deliberately rather than flick between.
   */
  foldOnPhone?: boolean;
}

const VIEW_TABS: ViewTab[] = [
  {
    value: "chat",
    label: "Chat",
    icon: MessageSquare,
    sessionOnly: true,
    chatOnly: true,
  },
  { value: "desktop", label: "Desktop", icon: MonitorPlay, sessionOnly: true, foldOnPhone: true },
  { value: "browser", label: "Browser", icon: Globe2, sessionOnly: true },
  { value: "changes", label: "Diff", icon: GitCompare, sessionOnly: true },
  { value: "files", label: "Files", icon: Files, sessionOnly: true, foldOnPhone: true },
  {
    value: "details",
    label: "Details",
    icon: SlidersHorizontal,
    sessionOnly: false,
    foldOnPhone: true,
  },
];

/** A secondary control: an icon button while there is room, a menu item otherwise. */
interface SecondaryControl {
  key: string;
  label: string;
  tooltip?: string;
  icon: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  busy?: boolean;
  pressed?: boolean;
  /** Kept inline at every density — the primary desktop lifecycle action. */
  primary?: boolean;
}

export interface SpecTaskViewToolbarProps {
  currentView: TaskView;
  onViewChange: (view: TaskView | null) => void;
  /** Whether the task has a live session (gates the session-only tabs). */
  hasSession: boolean;
  /** Show the Chat tab (single-column layouts where chat has no panel). */
  showChatTab?: boolean;
  /** Headless tasks have no stream and cannot be converted to a desktop. */
  showDesktop?: boolean;
  /** Status-specific action buttons (Reject / Open PR / …). */
  renderActions?: (density: ToolbarDensity) => ReactNode;

  onToggleTerminal?: () => void;
  terminalOpen?: boolean;

  showStart?: boolean;
  onStart?: () => void;
  startBusy?: boolean;

  showStop?: boolean;
  onStop?: () => void;
  stopBusy?: boolean;

  showRestart?: boolean;
  onRestart?: () => void;
  restartBusy?: boolean;

  showKeepAlive?: boolean;
  keepAlive?: boolean;
  onToggleKeepAlive?: () => void;
  keepAliveBusy?: boolean;

  showUpload?: boolean;
  onUpload?: () => void;
  uploadBusy?: boolean;

  /** Restore a collapsed chat panel (split-view layouts only). */
  onRestoreSplit?: () => void;

  /** Extra items appended to the overflow menu (Share, Clone, …). */
  renderMenuItems?: (closeMenu: () => void) => ReactNode;

  /** Trailing control: collapse the content panel. Takes precedence over close. */
  onCollapsePanel?: () => void;
  /** Trailing control: close the task view. */
  onClosePanel?: () => void;
}

/**
 * The view-switcher toolbar shown above the task content, in both the split
 * view (right panel) and the single-column layout. Everything shrinks as the
 * toolbar narrows, and secondary controls fold into the overflow menu so the
 * row never wraps on tablet-width panels.
 */
const SpecTaskViewToolbar: React.FC<SpecTaskViewToolbarProps> = ({
  currentView,
  onViewChange,
  hasSession,
  showChatTab = false,
  showDesktop = true,
  renderActions,
  onToggleTerminal,
  terminalOpen,
  showStart,
  onStart,
  startBusy,
  showStop,
  onStop,
  stopBusy,
  showRestart,
  onRestart,
  restartBusy,
  showKeepAlive,
  keepAlive,
  onToggleKeepAlive,
  keepAliveBusy,
  showUpload,
  onUpload,
  uploadBusy,
  onRestoreSplit,
  renderMenuItems,
  onCollapsePanel,
  onClosePanel,
}) => {
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const closeMenu = () => setMenuAnchorEl(null);

  // Density comes from the toolbar's own width, not the viewport: in split view
  // the toolbar lives in the right panel, which is far narrower than the window.
  const isPhone = useIsPhone();
  const [toolbarRef, toolbarWidth] = useElementWidth<HTMLDivElement>();
  const density = toolbarDensityForWidth(toolbarWidth);

  const tab = TAB_METRICS[density];
  const iconButtonSx = toolbarIconButtonSx(density);
  const controlIconSize = ICON_BUTTON_METRICS[density].icon;

  const availableTabs = VIEW_TABS.filter(
    (t) => (!t.sessionOnly || hasSession)
      && (!t.chatOnly || showChatTab)
      && (t.value !== "desktop" || showDesktop),
  );
  const tabs = availableTabs.filter((t) => !(isPhone && t.foldOnPhone));
  const foldedTabs = availableTabs.filter((t) => isPhone && t.foldOnPhone);

  const controls: SecondaryControl[] = [];
  if (onRestoreSplit) {
    controls.push({
      key: "restore-split",
      label: "Restore split view",
      icon: <PanelLeft size={controlIconSize} />,
      onClick: onRestoreSplit,
    });
  }
  if (onToggleTerminal) {
    controls.push({
      key: "terminal",
      label: "Terminal",
      tooltip: "Toggle terminal drawer (Ctrl/Cmd+J)",
      icon: <PanelBottom size={controlIconSize} />,
      onClick: onToggleTerminal,
      pressed: terminalOpen,
    });
  }
  if (showStart && onStart) {
    controls.push({
      key: "start",
      label: "Start desktop",
      icon: <Play size={controlIconSize} />,
      onClick: onStart,
      disabled: startBusy,
      busy: startBusy,
      primary: true,
    });
  }
  if (showStop && onStop) {
    controls.push({
      key: "stop",
      label: "Stop desktop",
      icon: <Square size={controlIconSize} fill="currentColor" />,
      onClick: onStop,
      disabled: stopBusy,
      busy: stopBusy,
      primary: true,
    });
  }
  if (showRestart && onRestart) {
    controls.push({
      key: "restart",
      label: "Restart agent session",
      icon: <RotateCw size={controlIconSize} />,
      onClick: onRestart,
      disabled: restartBusy,
      busy: restartBusy,
    });
  }
  if (showKeepAlive && onToggleKeepAlive) {
    controls.push({
      key: "keep-alive",
      label: keepAlive ? "Keep Alive ON" : "Keep Alive OFF",
      tooltip: keepAlive
        ? "Keep Alive ON — won't auto-sleep"
        : "Keep Alive OFF — will auto-sleep when idle",
      icon: keepAlive ? (
        <Lock size={controlIconSize} />
      ) : (
        <LockOpen size={controlIconSize} />
      ),
      onClick: onToggleKeepAlive,
      disabled: keepAliveBusy,
      pressed: keepAlive,
    });
  }
  if (showUpload && onUpload) {
    controls.push({
      key: "upload",
      label: "Upload files to sandbox",
      icon: <CloudUpload size={controlIconSize} />,
      onClick: onUpload,
      disabled: uploadBusy,
      busy: uploadBusy,
    });
  }

  // Below the roomiest density only the primary lifecycle control stays inline;
  // everything else moves into the vertical-dot menu.
  const keepInline =
    density === "comfortable" ? controls : controls.filter((c) => c.primary);
  const overflow = controls.filter((c) => !keepInline.includes(c));

  const extraMenuItems = renderMenuItems?.(closeMenu);
  const hasMenu = overflow.length > 0 || foldedTabs.length > 0 || !!extraMenuItems;

  return (
    <Box
      ref={toolbarRef}
      sx={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        flexWrap: "wrap",
        minWidth: 0,
        px: 1,
        pt: 1,
        pb: 0.5,
        minHeight: density === "comfortable" ? 53 : 45,
        flexShrink: 0,
        boxSizing: "border-box",
        borderBottom: "1px solid",
        borderColor: "divider",
        backgroundColor: "background.paper",
        gap: 0.5,
      }}
    >
      <ToggleButtonGroup
        value={currentView}
        exclusive
        onChange={(_, newView) => onViewChange(newView as TaskView | null)}
        size="small"
        sx={{
          flexShrink: 0,
          "& .MuiToggleButton-root": {
            width: tab.width,
            height: tab.height,
            minWidth: tab.width,
            p: 0,
            border: "none",
            borderRadius: "4px !important",
            textTransform: "none",
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            gap: 0.2,
            "&.Mui-selected": {
              backgroundColor: "action.selected",
            },
          },
        }}
      >
        {tabs.map(({ value, label, icon: Icon }) => (
          <ToggleButton
            key={value}
            value={value}
            aria-label={`${label} view`}
            // Tooltips only earn their keep once the label is gone.
            title={tab.showLabel ? undefined : label}
          >
            <Icon size={tab.icon} />
            {tab.showLabel && (
              <Typography
                sx={{
                  fontSize: density === "comfortable" ? "0.65rem" : "0.6rem",
                  lineHeight: 1,
                  fontWeight: 400,
                  textTransform: "none",
                }}
              >
                {label}
              </Typography>
            )}
          </ToggleButton>
        ))}
      </ToggleButtonGroup>

      {renderActions?.(density)}

      <Box sx={{ flex: 1, minWidth: 0 }} />

      <Box
        sx={{
          display: "flex",
          gap: 0.25,
          alignItems: "center",
          flexShrink: 0,
        }}
      >
        {keepInline.map((control) => (
          <Tooltip key={control.key} title={control.tooltip ?? control.label}>
            <span style={{ display: "inline-flex" }}>
              <IconButton
                size="small"
                aria-label={control.label}
                aria-pressed={control.pressed}
                onClick={control.onClick}
                disabled={control.disabled}
                sx={iconButtonSx}
              >
                {control.busy ? (
                  <CircularProgress size={controlIconSize} />
                ) : (
                  control.icon
                )}
              </IconButton>
            </span>
          </Tooltip>
        ))}

        {hasMenu && (
          <Tooltip title="More actions">
            <IconButton
              size="small"
              aria-label="More actions"
              onClick={(event) => setMenuAnchorEl(event.currentTarget)}
              sx={iconButtonSx}
            >
              <EllipsisVertical size={controlIconSize} />
            </IconButton>
          </Tooltip>
        )}

        {onCollapsePanel ? (
          <Tooltip title="Collapse task panel">
            <IconButton
              size="small"
              aria-label="Collapse task panel"
              onClick={onCollapsePanel}
              sx={iconButtonSx}
            >
              <PanelRight size={controlIconSize} />
            </IconButton>
          </Tooltip>
        ) : onClosePanel && !isPhone ? (
          <Tooltip title="Close">
            <IconButton
              size="small"
              aria-label="Close"
              onClick={onClosePanel}
              sx={iconButtonSx}
            >
              <X size={controlIconSize} />
            </IconButton>
          </Tooltip>
        ) : null}
      </Box>

      <Menu
        anchorEl={menuAnchorEl}
        open={Boolean(menuAnchorEl)}
        onClose={closeMenu}
      >
        {foldedTabs.map(({ value, label, icon: Icon }) => (
          <MenuItem
            key={`view-${value}`}
            selected={currentView === value}
            onClick={() => {
              closeMenu();
              onViewChange(value);
            }}
          >
            <ListItemIcon>
              <Icon size={18} />
            </ListItemIcon>
            <ListItemText>{label}</ListItemText>
          </MenuItem>
        ))}
        {overflow.map((control) => (
          <MenuItem
            key={control.key}
            disabled={control.disabled}
            onClick={() => {
              closeMenu();
              control.onClick();
            }}
          >
            <ListItemIcon>
              {control.busy ? <CircularProgress size={18} /> : control.icon}
            </ListItemIcon>
            <ListItemText>{control.label}</ListItemText>
          </MenuItem>
        ))}
        {extraMenuItems}
      </Menu>
    </Box>
  );
};

export default SpecTaskViewToolbar;
