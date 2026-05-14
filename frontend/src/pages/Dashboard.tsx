import ClearIcon from "@mui/icons-material/Clear";
import StorageIcon from "@mui/icons-material/Storage";
import WarningAmberIcon from "@mui/icons-material/WarningAmber";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Container from "@mui/material/Container";
import Divider from "@mui/material/Divider";
import FormControl from "@mui/material/FormControl";
import FormControlLabel from "@mui/material/FormControlLabel";
import FormGroup from "@mui/material/FormGroup";
import Grid from "@mui/material/Grid";
import IconButton from "@mui/material/IconButton";
import InputLabel from "@mui/material/InputLabel";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import React, { FC, useCallback, useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import LLMCallsTable from "../components/dashboard/LLMCallsTable";
import UsageDashboard from "../components/usage/UsageDashboard";
import Interaction from "../components/session/Interaction";
import SessionBadgeKey from "../components/session/SessionBadgeKey";
import SessionToolbar from "../components/session/SessionToolbar";
import SessionSummary from "../components/session/SessionSummary";
// Page wrapper removed - Dashboard is now rendered inside FullScreenDialog
import JsonWindowLink from "../components/widgets/JsonWindowLink";
import Window from "../components/widgets/Window";
import useAccount from "../hooks/useAccount";
import useApi from "../hooks/useApi";
import { ISessionSummary } from "../types";
import {
    TypesInteraction,
    TypesSession,
    TypesSessionSummary,
} from "../api/api";

import ProviderEndpointsTable from "../components/dashboard/ProviderEndpointsTable";
import OAuthProvidersTable from "../components/dashboard/OAuthProvidersTable";
import HelixModelsTable from "../components/dashboard/HelixModelsTable";
import RunnerProfilesTable from "../components/dashboard/RunnerProfilesTable";
import PricingTable from "../components/dashboard/PricingTable";
import SystemSettingsTable from "../components/dashboard/SystemSettingsTable";
import ServiceConnectionsTable from "../components/dashboard/ServiceConnectionsTable";
import AgentSandboxes from "../components/admin/AgentSandboxes";
import AdminOrgsTable from "../components/dashboard/AdminOrgsTable";
import UsersTable from "../components/dashboard/UsersTable";
import UserDetailPanel from "../components/dashboard/UserDetailPanel";
import KoditAdminTable from "../components/dashboard/KoditAdminTable";
import KoditAdminRepoDetail from "../components/dashboard/KoditAdminRepoDetail";
import KoditAdminQueue from "../components/dashboard/KoditAdminQueue";
import Chip from "@mui/material/Chip";
import LaunchIcon from "@mui/icons-material/Launch";
import Button from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";

const START_ACTIVE = true;

interface DashboardProps {
    tab?: string
}

const Dashboard: FC<DashboardProps> = ({ tab = "llm_calls" }) => {
    const account = useAccount();
    const api = useApi();

    const activeRef = useRef(START_ACTIVE);

    const [viewingSession, setViewingSession] = useState<TypesSession>();
    const [active, setActive] = useState(START_ACTIVE);
    const [sessionFilter, setSessionFilter] = useState("");
    const [selectedSandboxId, setSelectedSandboxId] = useState<string>("");
    const [sessionIdParam, setSessionIdParam] = useState<string>("");
    const [repoId, setRepoId] = useState<string>("");
    const [selectedUserId, setSelectedUserId] = useState<string>("");
    const apiClient = api.getApiClient();

    const session_id = sessionIdParam;

    // Fetch list of registered sandbox instances for the agent_sandboxes tab
    const { data: sandboxInstances, isLoading: isLoadingSandboxes } = useQuery({
        queryKey: ["sandbox-instances"],
        queryFn: async () => {
            const response = await apiClient.v1SandboxesList();
            return response.data;
        },
        enabled: tab === "agent_sandboxes",
        refetchInterval: 10000,
    });

    // Don't auto-select - default to "All Sandboxes" view for aggregate monitoring

    const onViewSession = useCallback((session_id: string) => {
        setSessionIdParam(session_id);
    }, []);

    const onCloseViewingSession = useCallback(() => {
        setViewingSession(undefined);
        setSessionIdParam("");
    }, []);

    useEffect(() => {
        if (!session_id) return;
        if (!account.user) return;
        const loadSession = async () => {
            const session = await api.get<TypesSession>(
                `/api/v1/sessions/${session_id}`,
            );
            if (!session) return;
            setViewingSession(session);
        };
        loadSession();
    }, [account.user, session_id]);

    const isLoadingDashboardData = false; // Sandbox-absorbs-runner: dashboardData removed

    const handleFilterChange = (event: React.ChangeEvent<HTMLInputElement>) => {
        setSessionFilter(event.target.value);
    };

    const clearFilter = () => {
        setSessionFilter("");
    };

    if (!account.user) return null;
    if (isLoadingDashboardData) return null;

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
            {/* Version info bar */}
            <Box
                sx={{
                    width: "100%",
                    display: "flex",
                    flexDirection: "row",
                    alignItems: "center",
                    justifyContent: "space-between",
                    px: 3,
                    py: 1,
                    borderBottom: '1px solid rgba(255, 255, 255, 0.06)',
                    flexShrink: 0,
                }}
            >
                <Box sx={{ flex: 1 }}></Box>
                {account.serverConfig.version && (
                    <Typography
                        variant="body2"
                        sx={{
                            color: "rgba(255, 255, 255, 0.7)",
                            textAlign: "center",
                        }}
                    >
                        Helix Control Plane version:{" "}
                        {account.serverConfig.version}
                    </Typography>
                )}
                <Box
                    sx={{
                        flex: 1,
                        display: "flex",
                        justifyContent: "flex-end",
                    }}
                >
                    {tab === "runners" && <SessionBadgeKey />}
                </Box>
            </Box>
            <Container
                maxWidth="xl"
                sx={{
                    mt: 4,
                    height: "100%",
                    display: "flex",
                    flexDirection: "column",
                }}
            >
                {(!tab || tab === "llm_calls") && (
                    <Box
                        sx={{
                            width: "100%",
                        }}
                    >
                        <Box
                            sx={{
                                mb: 2,
                                display: "flex",
                                alignItems: "center",
                            }}
                        >
                            <TextField
                                label="Filter by Session ID"
                                variant="outlined"
                                value={sessionFilter}
                                onChange={handleFilterChange}
                                sx={{ flexGrow: 1, mr: 1 }}
                            />
                            {sessionFilter && (
                                <IconButton onClick={clearFilter} size="small">
                                    <ClearIcon />
                                </IconButton>
                            )}
                        </Box>
                        <LLMCallsTable sessionFilter={sessionFilter} />
                    </Box>
                )}

                {tab === "usage" && (
                    <Box sx={{ width: "100%" }}>
                        <UsageDashboard />
                    </Box>
                )}

                {tab === "providers" && (
                    <Box
                        sx={{
                            width: "100%",
                        }}
                    >
                        <ProviderEndpointsTable />
                    </Box>
                )}

                {tab === "oauth_providers" && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <OAuthProvidersTable />
                    </Box>
                )}

                {tab === "service_connections" && account.admin && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <ServiceConnectionsTable />
                    </Box>
                )}

                {tab === "runners" && (
                    <Box sx={{ width: "100%", p: 3 }}>
                        <Typography variant="h5" gutterBottom>GPU Runners</Typography>
                        <Typography variant="body1" color="text.secondary" paragraph>
                            The legacy runner / scheduler / slot dashboard has been retired in
                            the sandbox-absorbs-runner pivot. What you used to see here now
                            lives in:
                        </Typography>
                        <ul>
                            <li><strong>Agent Sandboxes</strong> — connected sandboxes with their
                            GPU inventory, active profile, and per-service health.</li>
                            <li><strong>Runner Profiles</strong> — define and assign compose-based
                            profiles to sandboxes.</li>
                        </ul>
                    </Box>
                )}

                {tab === "helix_models" && (
                    <Box
                        sx={{
                            width: "100%",
                        }}
                    >
                        <HelixModelsTable />
                    </Box>
                )}

                {tab === "runner_profiles" && account.admin && (
                    <Box sx={{ width: "100%", p: 2 }}>
                        <RunnerProfilesTable />
                    </Box>
                )}

                {tab === "pricing" && account.admin && (
                    <Box
                        sx={{
                            width: "100%",
                        }}
                    >
                        <PricingTable />
                    </Box>
                )}

                {tab === "system_settings" && account.admin && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <SystemSettingsTable />
                    </Box>
                )}

                {tab === "agent_sandboxes" && account.admin && (
                    <Box sx={{ width: "100%" }}>
                        {/* Version Mismatch Alert - skip for dev builds (SHA1 hashes or unknown) */}
                        {(() => {
                            const controlPlaneVersion = account.serverConfig?.version;
                            if (!controlPlaneVersion || !sandboxInstances) return null;
                            
                            // Skip alert for dev builds: <unknown> or SHA1 hash (40 hex chars)
                            if (controlPlaneVersion === "<unknown>") return null;
                            const isSha1Hash = /^[a-f0-9]{40}$/i.test(controlPlaneVersion);
                            if (isSha1Hash) return null;
                            
                            const mismatchedSandboxes = sandboxInstances.filter(
                                (s) => s.helix_version && s.status === "online" && s.helix_version !== controlPlaneVersion
                            );
                            
                            if (mismatchedSandboxes.length === 0) return null;
                            
                            const formatVersion = (v: string) => v.length > 7 ? v.substring(0, 7) : v;
                            
                            return (
                                <Alert 
                                    severity="warning" 
                                    icon={<WarningAmberIcon />}
                                    sx={{ mb: 2 }}
                                >
                                    <strong>Version Mismatch:</strong> {mismatchedSandboxes.length} sandbox{mismatchedSandboxes.length > 1 ? "es" : ""} running different version than control plane (v{formatVersion(controlPlaneVersion)}):
                                    {" "}
                                    {mismatchedSandboxes.map((s, i) => (
                                        <span key={s.id}>
                                            {i > 0 && ", "}
                                            <strong>{s.hostname || s.id}</strong> (v{formatVersion(s.helix_version || "unknown")})
                                        </span>
                                    ))}
                                </Alert>
                            );
                        })()}
                        {/* Sandbox Selector */}
                        <Box sx={{ mb: 3, display: "flex", alignItems: "center", gap: 2, flexWrap: "wrap" }}>
                            <FormControl size="small" sx={{ minWidth: 400 }}>
                                <InputLabel id="sandbox-selector-label">Agent Sandbox</InputLabel>
                                <Select
                                    labelId="sandbox-selector-label"
                                    value={selectedSandboxId}
                                    label="Agent Sandbox"
                                    onChange={(e) => setSelectedSandboxId(e.target.value)}
                                    disabled={isLoadingSandboxes || !sandboxInstances?.length}
                                    startAdornment={<StorageIcon sx={{ mr: 1, color: "text.secondary" }} />}
                                >
                                    <MenuItem value="">
                                        <em>All Sandboxes</em>
                                    </MenuItem>
                                    {sandboxInstances?.map((instance) => {
                                        const version = instance.helix_version;
                                        const shortVersion = version && version.length > 7 ? version.substring(0, 7) : version;
                                        return (
                                            <MenuItem key={instance.id} value={instance.id}>
                                                {instance.gpu_vendor ? "🖥️" : "💻"}{" "}
                                                {instance.hostname || instance.id}
                                                {shortVersion && ` (v${shortVersion})`}
                                                {" "}- {instance.active_sandboxes || 0}/{instance.max_sandboxes || 10} sessions
                                            </MenuItem>
                                        );
                                    })}
                                </Select>
                            </FormControl>
                            {sandboxInstances && sandboxInstances.length > 0 && (
                                <Typography variant="body2" color="text.secondary">
                                    {sandboxInstances.length} sandbox{sandboxInstances.length !== 1 ? "es" : ""} registered
                                </Typography>
                            )}
                            {!isLoadingSandboxes && (!sandboxInstances || sandboxInstances.length === 0) && (
                                <Typography variant="body2" color="warning.main">
                                    No agent sandboxes registered
                                </Typography>
                            )}
                        </Box>
                        <AgentSandboxes selectedSandboxId={selectedSandboxId} />
                    </Box>
                )}

                {tab === "users" && account.admin && !selectedUserId && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <UsersTable onSelectUser={(id) => setSelectedUserId(id)} />
                    </Box>
                )}

                {tab === "users" && account.admin && selectedUserId && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <UserDetailPanel
                            userId={selectedUserId}
                            onBack={() => setSelectedUserId("")}
                        />
                    </Box>
                )}

                {tab === "orgs" && account.admin && (
                    <Box
                        sx={{
                            width: "100%",
                            p: 2,
                        }}
                    >
                        <AdminOrgsTable />
                    </Box>
                )}

                {tab === "kodit" && account.admin && !repoId && (
                    <Box sx={{ width: "100%", p: 2 }}>
                        <KoditAdminTable onViewDetail={(id) => setRepoId(id)} />
                    </Box>
                )}

                {tab === "kodit" && account.admin && repoId && (
                    <Box sx={{ width: "100%", p: 2 }}>
                        <KoditAdminRepoDetail koditRepoId={repoId} onBack={() => setRepoId("")} />
                    </Box>
                )}

                {tab === "kodit_queue" && account.admin && (
                    <Box sx={{ width: "100%", p: 2 }}>
                        <KoditAdminQueue />
                    </Box>
                )}

                {viewingSession && (
                    <Window
                        open
                        size="lg"
                        background="#FAEFE0"
                        withCancel
                        cancelTitle="Close"
                        onCancel={onCloseViewingSession}
                    >
                        <SessionToolbar session={viewingSession} />
                        {viewingSession.interactions?.map(
                            (interaction: TypesInteraction, i: number) => {
                                return (
                                    <Interaction
                                        key={i}
                                        serverConfig={account.serverConfig}
                                        interaction={interaction}
                                        session={viewingSession}
                                        onRegenerate={() => {}}
                                        onReloadSession={async () => {}}
                                        isOwner={true}
                                        isAdmin={false}
                                        session_id={viewingSession.id || ""}
                                        highlightAllFiles={false}
                                        isLastInteraction={
                                            i ===
                                            (viewingSession.interactions
                                                ?.length || 0) -
                                                1
                                        }
                                    />
                                );
                            },
                        )}
                    </Window>
                )}
            </Container>
        </Box>
    );
};

export default Dashboard;
