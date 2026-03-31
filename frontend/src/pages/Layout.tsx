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
import MuiSnackbar from "@mui/material/Snackbar";

import Sidebar from "../components/system/Sidebar";
import SessionsSidebar from "../components/session/SessionsSidebar";
import FilesSidebar from "../components/files/FilesSidebar";
import AdminPanelSidebar from "../components/admin/AdminPanelSidebar";
import AccountSidebar from "../components/account/AccountSidebar";
import OrgSidebar from "../components/orgs/OrgSidebar";
import AppSidebar from "../components/app/AppSidebar";
import ProjectsSidebar from "../components/project/ProjectsSidebar";
import ProjectSettingsSidebar from "../components/project/ProjectSettingsSidebar";
import FullScreenDialog from "../components/dialog/FullScreenDialog";
import Dashboard from "./Dashboard";
import Account from "./Account";
import ProjectSettings from "./ProjectSettings";
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

import { useFloatingRunnerState } from "../contexts/floatingRunnerState";
import FloatingModal from "../components/admin/FloatingModal";
import { useFloatingModal } from "../contexts/floatingModal";
import UserOrgSelector from "../components/orgs/UserOrgSelector";

import useRouter from "../hooks/useRouter";
import useAccount from "../hooks/useAccount";
import useLightTheme from "../hooks/useLightTheme";
import useThemeConfig from "../hooks/useThemeConfig";
import useIsBigScreen from "../hooks/useIsBigScreen";
import useApps from "../hooks/useApps";
import useUserMenuHeight from "../hooks/useUserMenuHeight";

// Admin and Connected Services are rendered as full-screen dialog overlays
// so the user stays within their current org-scoped URL
const SettingsDialogs: FC = () => {
  const { activeDialog, dialogOptions, closeDialog } = useSettingsDialog()
  const [adminTab, setAdminTab] = useState('llm_calls')
  const [accountTab, setAccountTab] = useState('general')
  const [projectSettingsTab, setProjectSettingsTab] = useState('general')

  // When opening the admin dialog with a specific tab, set it
  React.useEffect(() => {
    if (activeDialog === 'admin' && dialogOptions.tab) {
      setAdminTab(dialogOptions.tab)
    }
  }, [activeDialog, dialogOptions.tab])

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
      setAccountTab('general')
      setProjectSettingsTab('general')
    }
  }, [activeDialog])

  // Sync admin tab to URL so refresh preserves the current tab
  const handleAdminTabChange = React.useCallback((tab: string) => {
    setAdminTab(tab)
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
            <Dashboard tab={adminTab} />
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
      <DarkDialog
        open={activeDialog === 'account'}
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
            Account
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
              <AccountSidebar activeTab={accountTab} onTabChange={setAccountTab} />
            </Box>
            <Box sx={{ flex: 1, overflow: 'auto' }}>
              <Account tab={accountTab} />
            </Box>
          </Box>
        </DialogContent>
      </DarkDialog>
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
  const router = useRouter();
  const account = useAccount();
  const apps = useApps();
  const floatingRunnerState = useFloatingRunnerState();
  const floatingModal = useFloatingModal();
  const [showVersionBanner, setShowVersionBanner] = useState(true);
  const [licenseGracePeriodExpired, setLicenseGracePeriodExpired] =
    useState(false);
  const licenseTimerRef = useRef<NodeJS.Timeout | null>(null);
  const userMenuHeight = useUserMenuHeight();
  // Check if license is required (not mac-desktop AND (invalid license OR unknown deployment))
  const licenseRequired = useMemo(() => {
    return (
      account.serverConfig?.edition !== "mac-desktop" &&
      ((account.serverConfig?.license && !account.serverConfig.license.valid) ||
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

  // Hide sidebar on /new page when app_id is specified, otherwise use router.meta.drawer
  const shouldShowSidebar =
    router.meta.drawer &&
    !(router.name === "org_new" && router.params.app_id);

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
   * - default: Shows SessionsSidebar for most routes
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
        return <ProjectsSidebar />;

      case "org_agent":
        // Individual app pages use the new context sidebar for agent navigation
        return <AppSidebar />;

      case "org_settings":
      case "org_people":
      case "org_teams":
      case "org_billing":
      case "org_api_keys":
      case "team_people":
        // Organization management pages use the org context sidebar
        return <OrgSidebar />;

      case "files":
        return <FilesSidebar onOpenFile={() => {}} />;

      default:
        // Default to SessionsMenu for most routes
        return <SessionsSidebar onOpenSession={onOpenSession} />;
    }
  }

  // Fullscreen mode: render children without any chrome (sidebar, drawer, banners)
  if (router.meta.fullscreen) {
    return (
      <>
        {children}
        <Snackbar />
        <GlobalLoading />
        {licenseRequired && (
          <LicenseKeyPrompt gracePeriodExpired={licenseGracePeriodExpired} />
        )}
      </>
    );
  }

  return (
    <>
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
      <Box
        id="root-container"
        sx={{
          minHeight: "100dvh",
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
              background: lightTheme.backgroundColor,
              backgroundColor: lightTheme.backgroundColor,
              // For mobile (temporary), let MUI handle positioning (fixed)
              // For desktop (permanent), use relative positioning
              position: isBigScreen ? "relative" : undefined,
              whiteSpace: "nowrap",
              width: shouldShowSidebar
                ? isBigScreen
                  ? themeConfig.drawerWidth
                  : themeConfig.smallDrawerWidth
                : 64,
              boxSizing: "border-box",
              overflowX: "hidden", // Prevent horizontal scrolling
              // Mobile gets full height, desktop respects user menu
              // Use dvh (dynamic viewport height) for iOS Safari compatibility
              height: isBigScreen
                ? userMenuHeight > 0
                  ? `calc(100dvh - ${userMenuHeight}px)`
                  : "100%"
                : "100dvh",
              overflowY: "auto", // Both columns scroll together
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
              width: "100%",
            }}
          >
            {/* Always show UserOrgSelector - it will handle compact/expanded modes internally */}
            <Box
              sx={{
                minWidth: 64,
                width: 64,
                maxWidth: 64,
                minHeight: "fit-content", // Natural height based on content
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "flex-start",
                zIndex: 2,
                py: 0,
                ...(shouldShowSidebar
                  ? {
                      // Only show border when sidebar is visible
                      borderRight: lightTheme.border,
                    }
                  : {
                      // When sidebar is hidden, no border and background
                      bgcolor: lightTheme.backgroundColor,
                    }),
              }}
            >
              <UserOrgSelector sidebarVisible={shouldShowSidebar} />
            </Box>
            {shouldShowSidebar && (
              <Box
                sx={{
                  flex: 1,
                  minWidth: 0,
                  minHeight: "fit-content", // Natural height based on content
                  display: "flex",
                  flexDirection: "column",
                }}
              >
                <Sidebar userMenuHeight={userMenuHeight}>{sidebarMenu}</Sidebar>
              </Box>
            )}
          </Box>
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
          }}
        >
          <Box
            component="div"
            sx={{
              flexGrow: 1,
              backgroundColor:
                theme.palette.mode === "light"
                  ? themeConfig.lightBackgroundColor
                  : themeConfig.darkBackgroundColor,
              height: "100%",
              minHeight: "100%",
              minWidth: 0,
              overflow: "hidden",
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
    </>
  );
};

const LayoutWithDialogs: FC<{ children: ReactNode }> = ({ children }) => (
  <SettingsDialogProvider>
    <Layout>{children}</Layout>
  </SettingsDialogProvider>
)

export default LayoutWithDialogs;
