import React, {
  FC,
  useState,
  useMemo,
  ReactNode,
  useEffect,
  useRef,
} from "react";
import { useTheme } from "@mui/material/styles";
import CssBaseline from "@mui/material/CssBaseline";
import Typography from "@mui/material/Typography";
import Box from "@mui/material/Box";
import Drawer from "@mui/material/Drawer";
import Alert from "@mui/material/Alert";
import Button from "@mui/material/Button";
import MuiSnackbar from "@mui/material/Snackbar";
import { useDetectLocalProviders, useListProviders } from "../services/providersService";

import Sidebar from "../components/system/Sidebar";
import ProjectChatSidebar from "../components/session/ProjectChatSidebar";
import FilesSidebar from "../components/files/FilesSidebar";
import AdminPanelSidebar from "../components/admin/AdminPanelSidebar";
import OrgSidebar from "../components/orgs/OrgSidebar";
import AppSidebar from "../components/app/AppSidebar";
import ProjectSettingsSidebar from "../components/project/ProjectSettingsSidebar";
import FullScreenDialog from "../components/dialog/FullScreenDialog";
import Dashboard from "./Dashboard";
import ProjectSettings from "./ProjectSettings";
import AccountDialog from "../components/account/AccountDialog";
import OAuthConnections from "../components/account/OAuthConnections";
import { SettingsDialogProvider, useSettingsDialog } from "../contexts/settingsDialog";

import Snackbar from "../components/system/Snackbar";
import GlobalLoading from "../components/system/GlobalLoading";
import InstallPWA from "../components/system/InstallPWA";
import DarkDialog from "../components/dialog/DarkDialog";
import DialogContent from "@mui/material/DialogContent";
import IconButton from "@mui/material/IconButton";
import CloseIcon from "@mui/icons-material/Close";
import { LicenseKeyPrompt } from "../components/LicenseKeyPrompt";

import FloatingModal from "../components/admin/FloatingModal";
import { useFloatingModal } from "../contexts/floatingModal";
import UserOrgSelector from "../components/orgs/UserOrgSelector";
import {
  CHAT_SIDEBAR_DEFAULT_WIDTH,
  CHAT_SIDEBAR_MAX_WIDTH,
  CHAT_SIDEBAR_MIN_WIDTH,
  chatSidebarWidthStorageKey,
  clampChatSidebarWidth,
  parseChatSidebarWidth,
} from "../components/session/chatSidebarWidth";

import useRouter from "../hooks/useRouter";
import useAccount from "../hooks/useAccount";
import useLightTheme from "../hooks/useLightTheme";
import useThemeConfig from "../hooks/useThemeConfig";
import useIsBigScreen from "../hooks/useIsBigScreen";
import useIsPhone from "../hooks/useIsPhone";
import { isNavigationRouteActive } from "../components/orgs/UserOrgSelector.logic";
import useApps from "../hooks/useApps";
import useUserMenuHeight from "../hooks/useUserMenuHeight";
import { LIGHT_SIDEBAR_COLORS } from "../styles/themeTokens";
import { TOOLBAR_HEIGHT } from "../config";
import { usesFocusedAgentDetails } from "../utils/apps";
import { ChatSidebarProvider } from "../contexts/chatSidebar";
import {
  chatSidebarCollapsedStorageKey,
  parseChatSidebarCollapsed,
} from "../components/session/chatSidebarVisibility";

// Admin and Connected Services are rendered as full-screen dialog overlays
// so the user stays within their current org-scoped URL
const SettingsDialogs: FC = () => {
  const { activeDialog, dialogOptions, closeDialog } = useSettingsDialog()
  const [adminTab, setAdminTab] = useState('llm_calls')
  const [adminProviderId, setAdminProviderId] = useState<string | undefined>()
  const [projectSettingsTab, setProjectSettingsTab] = useState('general')

  // When opening the admin dialog with a specific tab, set it
  React.useEffect(() => {
    if (activeDialog === 'admin' && dialogOptions.tab) {
      setAdminTab(dialogOptions.tab)
      setAdminProviderId(dialogOptions.tab === 'providers' ? dialogOptions.providerId : undefined)
    }
  }, [activeDialog, dialogOptions.providerId, dialogOptions.tab])

  // When opening project settings with a specific tab, set it
  React.useEffect(() => {
    if (activeDialog === 'project-settings' && dialogOptions.tab) {
      setProjectSettingsTab(dialogOptions.tab)
    }
  }, [activeDialog, dialogOptions.tab])

  // Reset tabs when dialog closes
  React.useEffect(() => {
    if (!activeDialog) {
      setAdminTab('llm_calls')
      setAdminProviderId(undefined)
      setProjectSettingsTab('general')
    }
  }, [activeDialog])

  // Sync admin tab to URL so refresh preserves the current tab
  const handleAdminTabChange = React.useCallback((tab: string) => {
    setAdminTab(tab)
    if (tab !== 'providers') {
      setAdminProviderId(undefined)
    }
    const url = new URL(window.location.href)
    url.searchParams.set('dialog_tab', tab)
    if (tab !== 'providers') {
      url.searchParams.delete('dialog_provider_id')
      url.searchParams.delete('dialog_provider_from')
      url.searchParams.delete('dialog_provider_to')
    }
    window.history.replaceState({}, '', url.toString())
  }, [])

  const handleAdminProviderChange = React.useCallback((providerId?: string) => {
    setAdminProviderId(providerId)
    const url = new URL(window.location.href)
    if (providerId) {
      url.searchParams.set('dialog_provider_id', providerId)
    } else {
      url.searchParams.delete('dialog_provider_id')
      url.searchParams.delete('dialog_provider_from')
      url.searchParams.delete('dialog_provider_to')
    }
    window.history.replaceState({}, '', url.toString())
  }, [])

  // Sync the account tab to URL so refresh and deep links preserve the section
  const handleAccountTabChange = React.useCallback((tab: string) => {
    const url = new URL(window.location.href)
    url.searchParams.set('dialog_tab', tab)
    window.history.replaceState({}, '', url.toString())
  }, [])

  // Sync project settings tab to URL
  const handleProjectSettingsTabChange = React.useCallback((tab: string) => {
    setProjectSettingsTab(tab)
    const url = new URL(window.location.href)
    url.searchParams.set('dialog_tab', tab)
    window.history.replaceState({}, '', url.toString())
  }, [])

  return (
    <>
      <FullScreenDialog
        open={activeDialog === 'admin'}
        onClose={closeDialog}
        title="Admin Panel"
      >
        <Box sx={{ display: 'flex', height: '100%', overflow: 'hidden' }}>
          <Box sx={{
            width: 240,
            flexShrink: 0,
            borderRight: '1px solid rgba(255, 255, 255, 0.1)',
            overflowY: 'auto',
          }}>
            <AdminPanelSidebar activeTab={adminTab} onTabChange={handleAdminTabChange} />
          </Box>
          <Box sx={{ flex: 1, overflow: 'auto' }}>
            <Dashboard
              tab={adminTab}
              initialSessionFilter={dialogOptions.sessionFilter}
              providerId={adminProviderId}
              onProviderChange={handleAdminProviderChange}
            />
          </Box>
        </Box>
      </FullScreenDialog>
      <FullScreenDialog
        open={activeDialog === 'connected-services'}
        onClose={closeDialog}
        title="Connected Services"
      >
        <Box sx={{ p: 3 }}>
          <OAuthConnections />
        </Box>
      </FullScreenDialog>
      <AccountDialog
        open={activeDialog === 'account'}
        onClose={closeDialog}
        initialTab={activeDialog === 'account' ? dialogOptions.tab : undefined}
        onTabChange={handleAccountTabChange}
      />
      {/* Project Settings Dialog */}
      <DarkDialog
        open={activeDialog === 'project-settings'}
        onClose={closeDialog}
        maxWidth="xl"
        fullWidth
        PaperProps={{
          sx: {
            height: '90vh',
            maxHeight: '90vh',
          },
        }}
      >
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            px: 3,
            py: 1.5,
            flexShrink: 0,
          }}
        >
          <Typography variant="h6" sx={{ fontWeight: 600 }}>
            Project Settings
          </Typography>
          <IconButton
            onClick={closeDialog}
            sx={{
              color: '#A0AEC0',
              '&:hover': {
                color: '#F1F1F1',
                backgroundColor: 'rgba(255, 255, 255, 0.08)',
              },
            }}
          >
            <CloseIcon />
          </IconButton>
        </Box>
        <DialogContent sx={{ p: 0, display: 'flex', overflow: 'hidden' }}>
          <Box sx={{ display: 'flex', height: '100%', width: '100%' }}>
            <Box sx={{
              width: 240,
              flexShrink: 0,
              borderRight: '1px solid rgba(255, 255, 255, 0.1)',
              overflowY: 'auto',
              pr: 1,
            }}>
              <ProjectSettingsSidebar activeTab={projectSettingsTab as any} onTabChange={handleProjectSettingsTabChange as any} />
            </Box>
            <Box sx={{ flex: 1, overflow: 'auto' }}>
              {dialogOptions.projectId && (
                <ProjectSettings projectId={dialogOptions.projectId} tab={projectSettingsTab} />
              )}
            </Box>
          </Box>
        </DialogContent>
      </DarkDialog>
    </>
  )
}

const Layout: FC<{
  children: ReactNode;
}> = ({ children }) => {
  const theme = useTheme();
  const themeConfig = useThemeConfig();
  const lightTheme = useLightTheme();
  const isBigScreen = useIsBigScreen();
  // A phone has no room for the 300px drawer to sit beside the conversation —
  // the chat list ends up a strip too narrow to read. It takes the screen.
  const isPhone = useIsPhone();
  const router = useRouter();
  const account = useAccount();
  const apps = useApps();
  const floatingModal = useFloatingModal();
  const orgId = router.params.org_id || "";
  const sidebarWidthStorageKey = chatSidebarWidthStorageKey(orgId);
  const sidebarCollapsedStorageKey = chatSidebarCollapsedStorageKey(orgId);
  const defaultChatSidebarWidth = themeConfig.drawerWidth || CHAT_SIDEBAR_DEFAULT_WIDTH;
  const [chatSidebarWidth, setChatSidebarWidth] = useState(() => {
    try {
      return parseChatSidebarWidth(
        window.localStorage.getItem(sidebarWidthStorageKey),
        defaultChatSidebarWidth,
      );
    } catch {
      return clampChatSidebarWidth(defaultChatSidebarWidth);
    }
  });
  const [isResizingChatSidebar, setIsResizingChatSidebar] = useState(false);
  const [chatSidebarCollapsed, setChatSidebarCollapsed] = useState(() => {
    try {
      return parseChatSidebarCollapsed(window.localStorage.getItem(sidebarCollapsedStorageKey));
    } catch {
      return false;
    }
  });
  const chatSidebarWidthRef = useRef(chatSidebarWidth);
  const [showVersionBanner, setShowVersionBanner] = useState(true);
  const [showLocalProviderBanner, setShowLocalProviderBanner] = useState(true);
  const { data: detectedProviders } = useDetectLocalProviders(!!account.user);
  const { data: allProviders } = useListProviders({ enabled: !!account.user });
  const unconnectedLocal = useMemo(() => {
    if (!detectedProviders || !allProviders) return [];
    return detectedProviders.filter(
      dp => !allProviders.some(e => e.name === dp.server_type)
    );
  }, [detectedProviders, allProviders]);
  const [licenseGracePeriodExpired, setLicenseGracePeriodExpired] =
    useState(false);
  const licenseTimerRef = useRef<NodeJS.Timeout | null>(null);
  const userMenuHeight = useUserMenuHeight();

  useEffect(() => {
    try {
      setChatSidebarCollapsed(parseChatSidebarCollapsed(
        window.localStorage.getItem(sidebarCollapsedStorageKey),
      ));
    } catch {
      setChatSidebarCollapsed(false);
    }
  }, [sidebarCollapsedStorageKey]);

  const setChatSidebarCollapsedAndPersist = (collapsed: boolean) => {
    setChatSidebarCollapsed(collapsed);
    try {
      window.localStorage.setItem(sidebarCollapsedStorageKey, String(collapsed));
    } catch {
      // Persistence is optional when browser storage is unavailable.
    }
  };

  useEffect(() => {
    try {
      const storedWidth = window.localStorage.getItem(sidebarWidthStorageKey);
      const nextWidth = parseChatSidebarWidth(storedWidth, defaultChatSidebarWidth);
      chatSidebarWidthRef.current = nextWidth;
      setChatSidebarWidth(nextWidth);
    } catch {
      const nextWidth = clampChatSidebarWidth(defaultChatSidebarWidth);
      chatSidebarWidthRef.current = nextWidth;
      setChatSidebarWidth(nextWidth);
    }
  }, [sidebarWidthStorageKey, defaultChatSidebarWidth]);

  useEffect(() => {
    chatSidebarWidthRef.current = chatSidebarWidth;
  }, [chatSidebarWidth]);

  useEffect(() => {
    if (!isResizingChatSidebar) return;

    const handlePointerMove = (event: PointerEvent) => {
      const nextWidth = clampChatSidebarWidth(event.clientX, defaultChatSidebarWidth);
      chatSidebarWidthRef.current = nextWidth;
      setChatSidebarWidth(nextWidth);
    };
    const handlePointerUp = () => {
      try {
        window.localStorage.setItem(sidebarWidthStorageKey, String(chatSidebarWidthRef.current));
      } catch {
        // Persistence is optional when browser storage is unavailable.
      }
      setIsResizingChatSidebar(false);
    };

    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp);
    return () => {
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
    };
  }, [defaultChatSidebarWidth, isResizingChatSidebar, sidebarWidthStorageKey]);

  const persistChatSidebarWidth = (nextWidth: number) => {
    const width = clampChatSidebarWidth(nextWidth, defaultChatSidebarWidth);
    chatSidebarWidthRef.current = width;
    setChatSidebarWidth(width);
    try {
      window.localStorage.setItem(sidebarWidthStorageKey, String(width));
    } catch {
      // Persistence is optional when browser storage is unavailable.
    }
  };
  // Check if license is required (not mac-desktop AND (invalid license OR unknown deployment))
  const licenseRequired = useMemo(() => {
    return (
      account.serverConfig?.edition !== "mac-desktop" &&
      ((account.license && !account.license.valid) ||
        account.serverConfig?.deployment_id === "unknown")
    );
  }, [account.serverConfig]);

  // Start 5-minute timer when license is required AND user is logged in
  // (we don't disable UI until they're logged in, so they have a chance to log in as admin)
  useEffect(() => {
    if (licenseRequired && account.user && !licenseGracePeriodExpired) {
      // Start 5-second grace period timer (for testing - change to 5 * 60 * 1000 for production)
      licenseTimerRef.current = setTimeout(() => {
        setLicenseGracePeriodExpired(true);
      }, 5 * 1000); // 5 seconds for testing

      return () => {
        if (licenseTimerRef.current) {
          clearTimeout(licenseTimerRef.current);
        }
      };
    } else if (!licenseRequired) {
      // License became valid, reset the expired state
      setLicenseGracePeriodExpired(false);
      if (licenseTimerRef.current) {
        clearTimeout(licenseTimerRef.current);
      }
    }
  }, [licenseRequired, account.user, licenseGracePeriodExpired]);

  const hasNewVersion = useMemo(() => {
    if (
      !account.serverConfig?.version ||
      !account.serverConfig?.latest_version
    ) {
      return false;
    }
    // Return false if version is "<unknown>"
    if (account.serverConfig.version === "<unknown>") {
      return false;
    }

    // Return false if version is a SHA1 hash (40 hex characters)
    const isSha1Hash = /^[a-f0-9]{40}$/i.test(account.serverConfig.version);
    if (isSha1Hash) {
      return false;
    }

    // Parse versions for comparison
    const parseVersion = (versionString: string) => {
      // Check if it's a pre-release version (contains hyphen)
      const isPreRelease = versionString.includes("-");

      // Extract base version and pre-release info
      let baseVersion = versionString;
      let preRelease = "";

      if (isPreRelease) {
        const parts = versionString.split("-");
        baseVersion = parts[0];
        preRelease = parts[1];
      }

      // Parse version numbers
      const versionParts = baseVersion
        .split(".")
        .map((part) => parseInt(part, 10));

      // Ensure we have a valid semver
      if (versionParts.length !== 3 || versionParts.some(isNaN)) {
        return null;
      }

      return {
        major: versionParts[0],
        minor: versionParts[1],
        patch: versionParts[2],
        isPreRelease,
        preRelease,
      };
    };

    const currentVersion = parseVersion(account.serverConfig.version);
    const latestVersion = parseVersion(account.serverConfig.latest_version);

    // If either version is invalid, fallback to simple comparison
    if (!currentVersion || !latestVersion) {
      return (
        account.serverConfig.version !== account.serverConfig.latest_version
      );
    }

    // Never show release candidates as updates (rc, alpha, beta, etc.)
    if (latestVersion.isPreRelease) {
      return false;
    }

    // Compare major, minor, patch
    if (currentVersion.major !== latestVersion.major) {
      return currentVersion.major < latestVersion.major;
    }
    if (currentVersion.minor !== latestVersion.minor) {
      return currentVersion.minor < latestVersion.minor;
    }
    if (currentVersion.patch !== latestVersion.patch) {
      return currentVersion.patch < latestVersion.patch;
    }

    // If we get here, the base versions are equal, so we need to check pre-release status
    // If current is pre-release and latest is not, then latest is newer
    if (currentVersion.isPreRelease && !latestVersion.isPreRelease) {
      return true;
    }

    // If latest is pre-release and current is not, then latest is not newer
    if (!currentVersion.isPreRelease && latestVersion.isPreRelease) {
      return false;
    }

    // If both are pre-release or both are not, use simple string comparison as fallback
    return account.serverConfig.version !== account.serverConfig.latest_version;
  }, [account.serverConfig?.version, account.serverConfig?.latest_version]);

  let sidebarMenu = null;
  const isOrgMenu = router.meta.menu == "orgs";

  // Determine which resource type to use
  // 1. Use resource_type from URL params if available
  // 2. If app_id is present in the URL, default to 'apps'
  // 3. Otherwise default to 'chat'
  const resourceType =
    router.params.resource_type || (router.params.app_id ? "apps" : "chat");

  // Hide secondary context sidebar on helix-org routes (nav is in the top
  // AppBar; chat is an in-page left rail). Still show the 64px org rail.
  // helix_org_chart is the exception: it is a leaf of the org settings nav
  // ("Org Chart"), so it keeps OrgSidebar.
  const isHelixOrgRoute =
    typeof router.name === "string" &&
    router.name.startsWith("helix_org_") &&
    router.name !== "helix_org_chart";
  const isProjectsIndex =
    router.name === "org_projects" &&
    (!router.params.tab || router.params.tab === "projects");
  const isConversationRoute = ["org_chat", "org_chat-task", "org_session", "org_new"].includes(
    router.name,
  );
  const routedAgent = router.params.app_id
    ? apps.apps.find((candidate) => candidate.id === router.params.app_id)
    : undefined;
  const isFocusedAgentRoute = router.name === "org_agent" && (
    !routedAgent || usesFocusedAgentDetails(routedAgent)
  );

  // Hide sidebar on /new page when app_id is specified, otherwise use router.meta.drawer
  const shouldShowSidebar =
    router.meta.drawer &&
    !isHelixOrgRoute &&
    !isProjectsIndex &&
    !isFocusedAgentRoute &&
    !(router.name === "org_new" && router.params.app_id);

  // On a phone the drawer holds the chat list, so opening a thread has to hide
  // it — otherwise the thread you just picked sits behind the list you picked
  // it from. Driven by the route rather than wired into each navigation, so it
  // holds however you arrive: tapping a row, sending from the composer, or a
  // link straight into a thread.
  useEffect(() => {
    if (!isPhone) return;
    if (isNavigationRouteActive(router.name, ["session", "chat-task"])) {
      account.setMobileMenuOpen(false);
    }
    // account is a context object and must not be a dependency.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isPhone, router.name]);

  if (shouldShowSidebar) {
    // Determine which sidebar to show based on route
    sidebarMenu = getSidebarForRoute(router.name, () => {
      account.setMobileMenuOpen(false);
    });
  }

  /**
   * Helper function to determine sidebar component based on route
   *
   * This flexible sidebar system allows different routes to show different sidebar content:
   * - 'app': Shows AppSidebar for agent navigation
   * - 'org_*': Shows OrgSidebar for organization management
   * - default: Shows ProjectChatSidebar for most routes
   *
   * To add a new context-specific sidebar:
   * 1. Create your sidebar component (e.g., FilesSidebar)
   * 2. Import it at the top of this file
   * 3. Add a new case in the switch statement below
   *
   * To disable sidebar for a route, return null instead of a component
   */
  function getSidebarForRoute(routeName: string, onOpenSession: () => void) {
    switch (routeName) {
      case "org_projects":
        return <OrgSidebar />;

      case "helix_org_root":
      case "helix_org_bots":
      case "helix_org_bot_detail":
      case "helix_org_human_detail":
      case "helix_org_settings":
      case "helix_org_topics":
      case "helix_org_assets":
      case "helix_org_topic_detail":
      case "helix_org_processor_detail":
        // Nav lives in the top AppBar (HelixOrgTopNav); chat is a left rail
        // inside HelixOrgShell — no middle ContextSidebar.
        return null;

      case "org_agent":
        // Individual app pages use the new context sidebar for agent navigation
        return <AppSidebar />;

      case "org_general":
      case "org_settings":
      case "org_people":
      case "org_teams":
      case "org_billing":
      case "org_usage":
      case "org_api_keys":
      case "org_providers":
      case "org_provider_detail":
      case "org_sandboxes":
      case "org_sandbox_detail":
      case "helix_org_chart":
      case "team_people":
        // Organization management pages use the org context sidebar
        return <OrgSidebar />;

      case "files":
        return <FilesSidebar onOpenFile={() => {}} />;

      default:
        return (
          <ProjectChatSidebar
            onCollapse={() => {
              if (isBigScreen) setChatSidebarCollapsedAndPersist(true);
              else account.setMobileMenuOpen(false);
            }}
            onOpenSession={onOpenSession}
          />
        );
    }
  }

  // Fullscreen mode: render children without any chrome (sidebar, drawer, banners).
  // Still include CssBaseline so MUI's typography + body styles apply — without it,
  // embed pages get browser defaults (Times New Roman, undefined text color → black
  // on whatever bg the page paints).
  if (router.meta.fullscreen) {
    return (
      <>
        <CssBaseline />
        {children}
        <Snackbar />
        <GlobalLoading />
        {licenseRequired && (
          <LicenseKeyPrompt gracePeriodExpired={licenseGracePeriodExpired} />
        )}
      </>
    );
  }

  const desktopChatSidebarCollapsed = isBigScreen && isConversationRoute && chatSidebarCollapsed;
  const visibleChatSidebarWidth = desktopChatSidebarCollapsed ? 64 : chatSidebarWidth;

  return (
    <ChatSidebarProvider
      collapsed={desktopChatSidebarCollapsed}
      collapse={() => setChatSidebarCollapsedAndPersist(true)}
      expand={() => setChatSidebarCollapsedAndPersist(false)}
    >
      <MuiSnackbar
        open={showVersionBanner && hasNewVersion}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          severity="info"
          onClose={() => setShowVersionBanner(false)}
          sx={{ width: "100%" }}
        >
          A new version of Helix ({account.serverConfig?.latest_version}) is
          available! You are running {account.serverConfig?.version}. Learn more{" "}
          <a
            style={{ color: "white" }}
            href={`https://github.com/helixml/helix/releases/${account.serverConfig?.latest_version}`}
            target="_blank"
            rel="noopener noreferrer"
          >
            here
          </a>
          .
        </Alert>
      </MuiSnackbar>
      <MuiSnackbar
        open={showLocalProviderBanner && unconnectedLocal.length > 0}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          severity="success"
          onClose={() => setShowLocalProviderBanner(false)}
          sx={{ width: "100%", bgcolor: "rgba(0,232,145,0.95)", color: "#000", "& .MuiAlert-icon": { color: "#000" } }}
          action={
            <Button
              size="small"
              onClick={() => {
                const orgId = account.organizationTools.organizations?.[0]?.id || account.organizationTools.organizations?.[0]?.name;
                if (orgId) router.navigate("org_providers", { org_id: orgId });
                setShowLocalProviderBanner(false);
              }}
              sx={{ color: "#000", fontWeight: 600, textTransform: "none", border: "1px solid rgba(0,0,0,0.3)", "&:hover": { bgcolor: "rgba(0,0,0,0.1)" } }}
            >
              Connect
            </Button>
          }
        >
          {unconnectedLocal.map(dp => dp.name).join(" and ")} detected on this machine with local AI models ready to use
        </Alert>
      </MuiSnackbar>
      <Box
        id="root-container"
        sx={{
          // The shell is exactly the fixed #root pane; a min-height here would
          // be a second source of overflow to fight with.
          height: "100%",
          display: "flex",
          backgroundColor: lightTheme.backgroundColor, // Extend background behind iOS safe area
          // Grey out UI when license grace period expires (only when logged in)
          ...(licenseRequired &&
            account.user &&
            licenseGracePeriodExpired && {
              filter: "grayscale(100%) brightness(0.5)",
              pointerEvents: "none",
              userSelect: "none",
            }),
        }}
        component="div"
      >
        <CssBaseline />
        <Drawer
          variant={isBigScreen ? "permanent" : "temporary"}
          open={isBigScreen || account.mobileMenuOpen}
          onClose={() => account.setMobileMenuOpen(false)}
          PaperProps={{
            sx: {
              background: lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.background : lightTheme.backgroundColor,
              backgroundColor: lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.background : lightTheme.backgroundColor,
              // For mobile (temporary), let MUI handle positioning (fixed)
              // For desktop (permanent), use relative positioning
              position: isBigScreen ? "relative" : undefined,
              whiteSpace: "nowrap",
              width: shouldShowSidebar
                ? isBigScreen
                  ? isConversationRoute
                    ? visibleChatSidebarWidth
                    : themeConfig.drawerWidth
                  : isPhone
                    ? "100%"
                    : themeConfig.smallDrawerWidth
                : 64,
              maxWidth: "100%",
              boxSizing: "border-box",
              overflowX: "hidden", // Prevent horizontal scrolling
              // Drawer takes full viewport height. The floating user menu is
              // rendered position: absolute INSIDE the Drawer (in the LEFT
              // rail), so shrinking the Drawer here would just leave a
              // visible gap below it. The shrink-by-userMenuHeight happens in
              // Sidebar.tsx for the secondary nav's content column only.
              // Use dvh (dynamic viewport height) for iOS Safari compatibility.
              height: isBigScreen ? "100%" : "min(var(--app-height, 100svh), 100svh)",
              // The primary rail must remain viewport-anchored. Secondary
              // navigation owns its scrolling inside SlideMenuContainer.
              overflowY: "hidden",
              display: "flex",
              flexDirection: "row",
              padding: 0,
            },
          }}
          sx={{
            height: "100%",
          }}
        >
          <Box
            sx={{
              display: "flex",
              flexDirection: "row",
              height: "100%",
              minHeight: 0,
              width: "100%",
              overflow: "hidden",
            }}
          >
            {/* Always show UserOrgSelector - it will handle compact/expanded modes internally */}
            <Box
              sx={{
                minWidth: 64,
                width: 64,
                maxWidth: 64,
                height: "100%",
                minHeight: 0,
                flexShrink: 0,
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "flex-start",
                zIndex: 2,
                py: 0,
                ...(shouldShowSidebar
                  ? {
                      borderRight: lightTheme.isLight ? `1px solid ${LIGHT_SIDEBAR_COLORS.border}` : lightTheme.border,
                      bgcolor: lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.background : lightTheme.backgroundColor,
                    }
                  : {
                      bgcolor: lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.background : lightTheme.backgroundColor,
                    }),
              }}
            >
              <UserOrgSelector
                sidebarVisible={shouldShowSidebar && !isConversationRoute}
              />
            </Box>
            {shouldShowSidebar && !desktopChatSidebarCollapsed && (
              <Box
                sx={{
                  flex: 1,
                  minWidth: 0,
                  height: "100%",
                  minHeight: 0,
                  overflow: "hidden",
                  display: "flex",
                  flexDirection: "column",
                }}
              >
                <Sidebar userMenuHeight={userMenuHeight}>{sidebarMenu}</Sidebar>
              </Box>
            )}
          </Box>
          {isBigScreen && shouldShowSidebar && isConversationRoute && !desktopChatSidebarCollapsed && (
            <Box
              data-chat-sidebar-resize-handle
              role="separator"
              aria-label="Resize chat sessions list"
              aria-orientation="vertical"
              aria-valuemin={CHAT_SIDEBAR_MIN_WIDTH}
              aria-valuemax={CHAT_SIDEBAR_MAX_WIDTH}
              aria-valuenow={chatSidebarWidth}
              tabIndex={0}
              onPointerDown={(event) => {
                if (event.button !== 0) return;
                event.preventDefault();
                setIsResizingChatSidebar(true);
              }}
              onKeyDown={(event) => {
                if (event.key === "ArrowLeft") {
                  event.preventDefault();
                  persistChatSidebarWidth(chatSidebarWidth - 16);
                } else if (event.key === "ArrowRight") {
                  event.preventDefault();
                  persistChatSidebarWidth(chatSidebarWidth + 16);
                } else if (event.key === "Home") {
                  event.preventDefault();
                  persistChatSidebarWidth(CHAT_SIDEBAR_MIN_WIDTH);
                } else if (event.key === "End") {
                  event.preventDefault();
                  persistChatSidebarWidth(CHAT_SIDEBAR_MAX_WIDTH);
                }
              }}
              sx={{
                position: "absolute",
                top: 0,
                right: 0,
                bottom: 0,
                zIndex: 3,
                width: 8,
                cursor: "col-resize",
                touchAction: "none",
                display: "flex",
                justifyContent: "flex-end",
                outline: "none",
                "&::after": {
                  content: '""',
                  display: "block",
                  width: isResizingChatSidebar ? 2 : 1,
                  height: "100%",
                  backgroundColor: isResizingChatSidebar
                    ? lightTheme.highlightColor
                    : "transparent",
                  transition: "background-color 120ms ease, width 120ms ease",
                },
                "&:hover::after, &:focus-visible::after": {
                  backgroundColor: lightTheme.isLight ? "rgba(0,0,0,0.28)" : "rgba(255,255,255,0.32)",
                },
              }}
            />
          )}
        </Drawer>
        <Box
          component="main"
          sx={{
            backgroundColor: (theme) => {
              if (router.meta.background) return router.meta.background;
              return lightTheme.backgroundColor;
            },
            flexGrow: 1,
            minWidth: 0,
            maxWidth: "100%",
            height: "100%",
            display: "flex",
            flexDirection: "column",
            overflow: "hidden",
            ...(isBigScreen && shouldShowSidebar && {
              "& [data-page-toolbar]": {
                height: TOOLBAR_HEIGHT,
                minHeight: TOOLBAR_HEIGHT,
              },
              "& [data-page-toolbar] > .MuiAppBar-root": {
                position: "fixed",
                left: isConversationRoute ? visibleChatSidebarWidth : 64,
                right: 0,
                width: "auto",
                zIndex: (theme) => theme.zIndex.drawer + 1,
              },
            }),
          }}
        >
          <Box
            component="div"
            sx={{
              flexGrow: 1,
              backgroundColor: lightTheme.backgroundColor,
              height: "100%",
              minHeight: 0,
              minWidth: 0,
              overflow: "hidden",
              // Flex column so full-height pages (helix-org shell, etc.) can
              // size their children with flex:1 / height:100%.
              display: "flex",
              flexDirection: "column",
            }}
          >
            {account.loggingOut ? (
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "center",
                  alignItems: "center",
                  height: "100%",
                }}
              >
                <Typography>Logging out...</Typography>
              </Box>
            ) : (
              children
            )}
          </Box>
        </Box>
        <Snackbar />
        <GlobalLoading />
        <InstallPWA />

        {/* Floating runner state disabled
          account.admin && floatingRunnerState.isVisible && (
            <FloatingRunnerState onClose={floatingRunnerState.hideFloatingRunnerState} />
          )
        */}
        {floatingModal.isVisible && account.admin && (
          <FloatingModal onClose={floatingModal.hideFloatingModal} />
        )}
        <SettingsDialogs />
        {/* Floating runner state toggle button disabled
          account.admin && (
            <Box
              sx={{
                position: 'fixed',
                bottom: 16,
                right: 16,
                zIndex: 9999,
              }}
            >
              <Tooltip title="Toggle floating runner state (Ctrl/Cmd+Shift+S)" arrow placement="left">
                <IconButton
                  onClick={(e) => {
                    const rect = e.currentTarget.getBoundingClientRect()
                    const clickPosition = {
                      x: rect.left - 340,
                      y: rect.top - 50
                    }
                    floatingRunnerState.toggleFloatingRunnerState(clickPosition)
                  }}
                  sx={{
                    width: 48,
                    height: 48,
                    backgroundColor: floatingRunnerState.isVisible ? '#00c8ff' : 'rgba(0, 200, 255, 0.1)',
                    backdropFilter: 'blur(10px)',
                    border: '1px solid rgba(0, 200, 255, 0.3)',
                    color: floatingRunnerState.isVisible ? '#000' : '#00c8ff',
                    boxShadow: '0 4px 12px rgba(0, 200, 255, 0.3)',
                    transition: 'all 0.2s ease',
                    '&:hover': {
                      backgroundColor: floatingRunnerState.isVisible ? '#00b3e6' : 'rgba(0, 200, 255, 0.2)',
                      transform: 'scale(1.05)',
                      boxShadow: '0 6px 16px rgba(0, 200, 255, 0.4)',
                    },
                    '&:active': {
                      transform: 'scale(0.95)',
                    }
                  }}
                >
                  <DnsIcon />
                </IconButton>
              </Tooltip>
            </Box>
          )
        */}
      </Box>
      {licenseRequired && (
        <LicenseKeyPrompt gracePeriodExpired={licenseGracePeriodExpired} />
      )}
    </ChatSidebarProvider>
  );
};

const LayoutWithDialogs: FC<{ children: ReactNode }> = ({ children }) => (
  <SettingsDialogProvider>
    <Layout>{children}</Layout>
  </SettingsDialogProvider>
)

export default LayoutWithDialogs;
